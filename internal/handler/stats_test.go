package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/stats"
)

type statsServiceStub struct {
	gotDays int
	report  *stats.Report
	err     error
}

func (s *statsServiceStub) Get(_ context.Context, days int) (*stats.Report, error) {
	s.gotDays = days
	return s.report, s.err
}

func TestServeStats(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, svc *statsServiceStub, query string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{resp: resp, l: zerolog.Nop()}
		h.SetStats(svc)
		target := "/stats"
		if query != "" {
			target += "?" + query
		}
		w := httptest.NewRecorder()
		h.ServeStats(w, httptest.NewRequest(http.MethodGet, target, nil))
		return w
	}

	report := &stats.Report{
		Days:        []stats.Day{{Date: "2026-10-25", Visits: 3}},
		TopProducts: []stats.Product{{ID: "widget", Title: "Widget", Views: 2}},
	}

	main.Run("DefaultDays", func(t *testing.T) {
		svc := &statsServiceStub{report: report}
		w := doRequest(t, svc, "")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 7, svc.gotDays)
		assert.JSONEq(t, `{"days":[{"date":"2026-10-25","visits":3}],
			"top_products":[{"id":"widget","title":"Widget","views":2}]}`, w.Body.String())
	})

	main.Run("Days30", func(t *testing.T) {
		svc := &statsServiceStub{report: report}
		w := doRequest(t, svc, "days=30")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 30, svc.gotDays)
	})

	main.Run("Upstream", func(t *testing.T) {
		svc := &statsServiceStub{err: fmt.Errorf("%w: x", stats.ErrUpstream)}
		w := doRequest(t, svc, "")
		assert.Equal(t, http.StatusBadGateway, w.Code)
		assert.JSONEq(t, `{"error":"bad gateway"}`, w.Body.String())
	})
}
