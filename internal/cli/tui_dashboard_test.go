package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func statsDays(n int) []StatsDay {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out := make([]StatsDay, n)
	for i := range out {
		out[i] = StatsDay{Date: start.AddDate(0, 0, i).Format("2006-01-02"), Visits: (i + 1) * 40}
	}
	return out
}

func TestDashboard(main *testing.T) {
	// start returns a model on the orders screen plus a pointer to the requested ?days= values.
	start := func(t *testing.T, notConfigured bool) (*tuiModel, *[]string) {
		var requested []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/stats" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			requested = append(requested, r.URL.Query().Get("days"))
			if notConfigured {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(Stats{
				Days:        statsDays(7),
				TopProducts: []StatsProduct{{ID: "mug", Title: "Clay mug", Views: 1214}, {ID: "bowl", Views: 97}},
			})
		}))
		t.Cleanup(srv.Close)
		m := newTUIModel(context.Background(), &Shop{Name: "test", URL: srv.URL}, NewClient(srv.URL, "k"))
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		return m, &requested
	}

	main.Run("TabOpensAndLoads", func(t *testing.T) {
		m, req := start(t, false)
		_, cmd := m.Update(key("tab"))
		require.Equal(t, screenDashboard, m.screen)
		runCmd(m, cmd)
		assert.Equal(t, []string{"7"}, *req)
		require.NotNil(t, m.stats)
		view := m.View()
		assert.Contains(t, view, "DASHBOARD 7d")
		assert.Contains(t, view, "Visits, last 7 days")
		assert.Contains(t, view, "Clay mug")
		assert.Contains(t, view, "<p> period")
	})

	main.Run("PeriodCycles", func(t *testing.T) {
		m, req := start(t, false)
		_, cmd := m.Update(key("tab"))
		runCmd(m, cmd)
		for i := 0; i < 3; i++ {
			_, cmd = m.Update(key("p"))
			runCmd(m, cmd)
		}
		assert.Equal(t, []string{"7", "14", "30", "7"}, *req)
		assert.Equal(t, 7, m.statsDays)
	})

	main.Run("RefreshReloads", func(t *testing.T) {
		m, req := start(t, false)
		_, cmd := m.Update(key("tab"))
		runCmd(m, cmd)
		_, cmd = m.Update(key("r"))
		runCmd(m, cmd)
		assert.Equal(t, []string{"7", "7"}, *req)
	})

	main.Run("TabBack", func(t *testing.T) {
		m, _ := start(t, false)
		_, cmd := m.Update(key("tab"))
		runCmd(m, cmd)
		m.Update(key("tab"))
		assert.Equal(t, screenOrders, m.screen)
		view := m.View()
		assert.NotContains(t, view, "DASHBOARD")
		assert.Contains(t, view, "<enter> open")
	})

	main.Run("NotConfigured", func(t *testing.T) {
		m, _ := start(t, true)
		_, cmd := m.Update(key("tab"))
		runCmd(m, cmd)
		assert.Contains(t, m.View(), "Stats are not configured on the server")
		assert.Empty(t, m.flash)
	})

	main.Run("RenderBarsHeights", func(t *testing.T) {
		out := renderBars([]StatsDay{
			{Date: "2026-10-19", Visits: 0}, {Date: "2026-10-20", Visits: 5}, {Date: "2026-10-21", Visits: 10},
		}, 40, 8)
		lines := strings.Split(out, "\n")
		rows := 0
		for _, l := range lines {
			if strings.Contains(l, "█") {
				rows++
			}
		}
		assert.Equal(t, 8, rows, "tallest bar spans the full height")
		// slot 8 -> bar width 7: 8 rows for the max, 4 for the half, none for zero.
		assert.Equal(t, (8+4)*7, strings.Count(out, "█"))
		assert.Contains(t, lines[0], "10")
		assert.Contains(t, out, "Mo")
	})

	main.Run("NarrowThirtyDays", func(t *testing.T) {
		out := renderBars(statsDays(30), 40, 8)
		require.NotEmpty(t, out)
		for _, l := range strings.Split(out, "\n") {
			assert.LessOrEqual(t, lipgloss.Width(l), 40, l)
		}
		assert.NotContains(t, out, "Mo")

		// 80 columns, 30 days: two-column slots, labels must be full two-digit days.
		wide := renderBars(statsDays(30), 80, 8)
		labels := strings.Split(wide, "\n")[9]
		assert.Contains(t, labels, "01")
		assert.Contains(t, labels, "03")
		assert.Contains(t, labels, "05")
		for _, l := range strings.Split(wide, "\n") {
			assert.LessOrEqual(t, lipgloss.Width(l), 80, l)
		}
		// 40 columns: one-column slots, every third day, still two digits.
		labels = strings.Split(out, "\n")[9]
		assert.Contains(t, labels, "01")
		assert.Contains(t, labels, "04")

		m, _ := start(t, false)
		m.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
		m.screen, m.statsDays = screenDashboard, 30
		m.stats = &Stats{Days: statsDays(30), TopProducts: []StatsProduct{{ID: "a-very-long-product-identifier", Views: 12345}}}
		for _, l := range strings.Split(m.View(), "\n") {
			assert.LessOrEqual(t, lipgloss.Width(l), 40, l)
		}
	})

	main.Run("TopProductsFallsBackToID", func(t *testing.T) {
		out := renderTopProducts([]StatsProduct{{ID: "id-of-mug", Title: "Clay mug", Views: 3}, {ID: "bowl-id", Views: 1}}, 60)
		assert.Contains(t, out, "Clay mug")
		assert.Contains(t, out, "bowl-id")
		assert.NotContains(t, out, "id-of-mug", "title shown instead of id when present")
		assert.Contains(t, out, "Views")
	})
}

func TestCompactCount(main *testing.T) {
	main.Run("fits the cell", func(t *testing.T) {
		assert.Equal(t, "183", compactCount(183, 3))
		assert.Equal(t, "1k", compactCount(1234, 3))
		assert.Equal(t, "12k", compactCount(12345, 3))
		assert.Equal(t, "12k", compactCount(12345, 4))
		assert.Equal(t, "", compactCount(123456, 3))
	})
}
