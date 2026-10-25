// Package stats builds a cached visit report from raw Cloudflare analytics.
package stats

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/ashep/simshop/internal/cloudflare"
)

const (
	cacheTTL      = 10 * time.Minute
	pathLimit     = 1000
	topProducts   = 10
	maxReportDays = 366
)

var productPathRe = regexp.MustCompile(`^/(?:[a-z]{2}/)?product/([^/]+)/?$`)

// ErrUpstream wraps any failure of the analytics source.
var ErrUpstream = errors.New("stats upstream failure")

// Source provides raw analytics data.
type Source interface {
	DailyVisits(ctx context.Context, from, to time.Time) ([]cloudflare.DayVisits, error)
	PathViews(ctx context.Context, from, to time.Time, limit int) ([]cloudflare.PathViews, error)
}

// Titler resolves a product title by id; unknown ids yield "".
type Titler interface {
	Title(id string) string
}

// Day is the number of visits on one UTC day.
type Day struct {
	Date   string `json:"date"`
	Visits int    `json:"visits"`
}

// Product is the number of views of one product page.
type Product struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Views int    `json:"views"`
}

// Report is the stats response body.
type Report struct {
	Days        []Day     `json:"days"`
	TopProducts []Product `json:"top_products"`
}

type cacheEntry struct {
	report  *Report
	expires time.Time
}

// Service builds and caches reports.
type Service struct {
	src    Source
	titles Titler
	now    func() time.Time

	mu    sync.Mutex
	cache map[int]cacheEntry
}

// NewService creates a Service.
func NewService(src Source, titles Titler, now func() time.Time) *Service {
	return &Service{src: src, titles: titles, now: now, cache: map[int]cacheEntry{}}
}

// Get returns the report for the last `days` UTC days including today.
func (s *Service) Get(ctx context.Context, days int) (*Report, error) {
	if days <= 0 || days > maxReportDays {
		return nil, fmt.Errorf("invalid days: %d", days)
	}

	now := s.now()
	s.mu.Lock()
	if e, ok := s.cache[days]; ok && now.Before(e.expires) {
		s.mu.Unlock()
		return e.report, nil
	}
	s.mu.Unlock()

	n := now.UTC()
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
	from := today.AddDate(0, 0, -(days - 1))
	to := today.AddDate(0, 0, 1)

	dv, err := s.src.DailyVisits(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	pv, err := s.src.PathViews(ctx, from, to, pathLimit)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUpstream, err)
	}

	byDate := make(map[string]int, len(dv))
	for _, d := range dv {
		byDate[d.Date] += d.Visits
	}
	report := &Report{Days: make([]Day, 0, days), TopProducts: []Product{}}
	for i := 0; i < days; i++ {
		date := from.AddDate(0, 0, i).Format("2006-01-02")
		report.Days = append(report.Days, Day{Date: date, Visits: byDate[date]})
	}

	views := map[string]int{}
	for _, p := range pv {
		if m := productPathRe.FindStringSubmatch(p.Path); m != nil {
			views[m[1]] += p.Views
		}
	}
	for id, v := range views {
		report.TopProducts = append(report.TopProducts, Product{ID: id, Title: s.titles.Title(id), Views: v})
	}
	sort.Slice(report.TopProducts, func(i, j int) bool {
		a, b := report.TopProducts[i], report.TopProducts[j]
		if a.Views != b.Views {
			return a.Views > b.Views
		}
		return a.ID < b.ID
	})
	if len(report.TopProducts) > topProducts {
		report.TopProducts = report.TopProducts[:topProducts]
	}

	s.mu.Lock()
	s.cache[days] = cacheEntry{report: report, expires: now.Add(cacheTTL)}
	s.mu.Unlock()
	return report, nil
}
