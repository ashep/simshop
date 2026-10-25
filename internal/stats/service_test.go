package stats_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/cloudflare"
	"github.com/ashep/simshop/internal/stats"
)

type fakeSource struct {
	days      []cloudflare.DayVisits
	paths     []cloudflare.PathViews
	err       error
	calls     int
	from, to  time.Time
	lastLimit int
}

func (f *fakeSource) DailyVisits(_ context.Context, from, to time.Time) ([]cloudflare.DayVisits, error) {
	f.calls++
	f.from, f.to = from, to
	return f.days, f.err
}

func (f *fakeSource) PathViews(_ context.Context, _, _ time.Time, limit int) ([]cloudflare.PathViews, error) {
	f.lastLimit = limit
	return f.paths, f.err
}

type fakeTitler map[string]string

func (f fakeTitler) Title(id string) string { return f[id] }

func TestService(main *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	main.Run("ZeroFills", func(t *testing.T) {
		src := &fakeSource{days: []cloudflare.DayVisits{{Date: "2026-10-20", Visits: 4}, {Date: "2026-10-25", Visits: 9}}}
		r, err := stats.NewService(src, fakeTitler{}, clock).Get(context.Background(), 7)
		require.NoError(t, err)
		require.Len(t, r.Days, 7)
		assert.Equal(t, "2026-10-19", r.Days[0].Date)
		assert.Equal(t, "2026-10-25", r.Days[6].Date)
		assert.Equal(t, 0, r.Days[0].Visits)
		assert.Equal(t, 4, r.Days[1].Visits)
		assert.Equal(t, 9, r.Days[6].Visits)
	})

	main.Run("Range", func(t *testing.T) {
		src := &fakeSource{}
		_, err := stats.NewService(src, fakeTitler{}, clock).Get(context.Background(), 7)
		require.NoError(t, err)
		assert.Equal(t, time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), src.from)
		assert.Equal(t, time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), src.to)
		assert.Equal(t, 1000, src.lastLimit)
	})

	main.Run("MergesLanguageVariants", func(t *testing.T) {
		src := &fakeSource{paths: []cloudflare.PathViews{
			{Path: "/uk/product/x", Views: 5}, {Path: "/en/product/x/", Views: 3}, {Path: "/product/x", Views: 1},
			{Path: "/catalog", Views: 50}, {Path: "/uk/", Views: 40},
		}}
		r, err := stats.NewService(src, fakeTitler{}, clock).Get(context.Background(), 7)
		require.NoError(t, err)
		require.Len(t, r.TopProducts, 1)
		assert.Equal(t, "x", r.TopProducts[0].ID)
		assert.Equal(t, 9, r.TopProducts[0].Views)
	})

	main.Run("TopTenSortedTiesById", func(t *testing.T) {
		var paths []cloudflare.PathViews
		for i := 0; i < 12; i++ {
			paths = append(paths, cloudflare.PathViews{Path: fmt.Sprintf("/product/p%02d", i), Views: 5})
		}
		paths = append(paths, cloudflare.PathViews{Path: "/product/big", Views: 100})
		r, err := stats.NewService(&fakeSource{paths: paths}, fakeTitler{}, clock).Get(context.Background(), 7)
		require.NoError(t, err)
		require.Len(t, r.TopProducts, 10)
		assert.Equal(t, "big", r.TopProducts[0].ID)
		assert.Equal(t, "p00", r.TopProducts[1].ID)
		assert.Equal(t, "p08", r.TopProducts[9].ID)
	})

	main.Run("TitlesAndUnknown", func(t *testing.T) {
		src := &fakeSource{paths: []cloudflare.PathViews{{Path: "/product/a", Views: 2}, {Path: "/product/gone", Views: 1}}}
		r, err := stats.NewService(src, fakeTitler{"a": "Alpha"}, clock).Get(context.Background(), 7)
		require.NoError(t, err)
		require.Len(t, r.TopProducts, 2)
		assert.Equal(t, "Alpha", r.TopProducts[0].Title)
		assert.Equal(t, "", r.TopProducts[1].Title)
	})

	main.Run("EmptyUpstream", func(t *testing.T) {
		r, err := stats.NewService(&fakeSource{}, fakeTitler{}, clock).Get(context.Background(), 7)
		require.NoError(t, err)
		require.NotNil(t, r.TopProducts)
		assert.Empty(t, r.TopProducts)
		require.Len(t, r.Days, 7)
		for _, d := range r.Days {
			assert.Equal(t, 0, d.Visits)
		}
	})

	main.Run("CachesTenMinutes", func(t *testing.T) {
		cur := now
		src := &fakeSource{}
		s := stats.NewService(src, fakeTitler{}, func() time.Time { return cur })
		_, _ = s.Get(context.Background(), 7)
		cur = cur.Add(9 * time.Minute)
		_, _ = s.Get(context.Background(), 7)
		assert.Equal(t, 1, src.calls)
		cur = cur.Add(2 * time.Minute)
		_, _ = s.Get(context.Background(), 7)
		assert.Equal(t, 2, src.calls)
	})

	main.Run("CacheKeyedByDays", func(t *testing.T) {
		src := &fakeSource{}
		s := stats.NewService(src, fakeTitler{}, clock)
		_, _ = s.Get(context.Background(), 7)
		r, _ := s.Get(context.Background(), 14)
		assert.Equal(t, 2, src.calls)
		assert.Len(t, r.Days, 14)
	})

	main.Run("ErrorNotCached", func(t *testing.T) {
		src := &fakeSource{err: errors.New("boom")}
		s := stats.NewService(src, fakeTitler{}, clock)
		_, err := s.Get(context.Background(), 7)
		require.ErrorIs(t, err, stats.ErrUpstream)
		_, err = s.Get(context.Background(), 7)
		require.ErrorIs(t, err, stats.ErrUpstream)
		assert.Equal(t, 2, src.calls)
	})

	main.Run("InvalidDays", func(t *testing.T) {
		s := stats.NewService(&fakeSource{}, fakeTitler{}, clock)
		for _, d := range []int{0, -3, 100000} {
			_, err := s.Get(context.Background(), d)
			require.Error(t, err)
			assert.NotErrorIs(t, err, stats.ErrUpstream)
		}
	})
}
