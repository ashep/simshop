package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCmd executes cmd and feeds the resulting message back into m until no command remains.
func runCmd(m *tuiModel, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		_, next := m.Update(msg)
		runCmd(m, next)
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEscape}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestTUI(main *testing.T) {
	orders := []Order{
		{ID: "019e9de8-c3c0-7000-8000-000000000001", Status: "paid", FirstName: "Ivan", LastName: "Petrenko",
			Phone: "+380501112233", City: "Kyiv", ProductID: "mug", Price: 45000, Currency: "uah",
			CreatedAt: "2026-10-20T10:00:00Z"},
		{ID: "019e9de8-c3c0-7000-8000-000000000002", Status: "processing", FirstName: "Olena", LastName: "Koval",
			Phone: "+380671234567", City: "Lviv", ProductID: "bowl", Price: 99000, Currency: "uah",
			CreatedAt: "2026-10-21T10:00:00Z"},
	}

	newServer := func(t *testing.T, patches *[]map[string]string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPatch {
				var body map[string]string
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				*patches = append(*patches, body)
				_ = json.NewEncoder(w).Encode(map[string]string{"status": body["status"]})
				return
			}
			_ = json.NewEncoder(w).Encode(orders)
		}))
		t.Cleanup(srv.Close)
		return srv
	}

	start := func(t *testing.T, patches *[]map[string]string) *tuiModel {
		srv := newServer(t, patches)
		m := newTUIModel(context.Background(), &Shop{Name: "test", URL: srv.URL}, NewClient(srv.URL, "k"))
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
		m.Init() // its batch includes the 30s refresh tick, so load directly instead
		runCmd(m, m.load())
		return m
	}

	main.Run("lists orders newest first", func(t *testing.T) {
		m := start(t, &[]map[string]string{})
		require.Len(t, m.visible, 2)
		assert.Equal(t, "processing", m.visible[0].Status)
		view := m.View()
		assert.Contains(t, view, "Olena Koval")
		assert.Contains(t, view, "Ivan Petrenko")
	})

	main.Run("search filters by every term", func(t *testing.T) {
		m := start(t, &[]map[string]string{})
		m.Update(key("/"))
		for _, r := range "kyiv ivan" {
			m.Update(key(string(r)))
		}
		m.Update(key("enter"))
		require.Len(t, m.visible, 1)
		assert.Equal(t, "Ivan Petrenko", customerName(m.visible[0]))
	})

	main.Run("ship via picker asks tracking then note and patches", func(t *testing.T) {
		var patches []map[string]string
		m := start(t, &patches)
		m.Update(key("s")) // cursor on the processing order
		require.Equal(t, modePicker, m.mode)
		require.Equal(t, "shipped", m.pickerItems[0])
		m.Update(key("enter"))
		require.Equal(t, modeInput, m.mode)
		for _, r := range "20450000000000" {
			m.Update(key(string(r)))
		}
		m.Update(key("enter"))
		require.Equal(t, modeInput, m.mode, "note prompt follows tracking")
		_, cmd := m.Update(key("enter"))
		runCmd(m, cmd)
		require.Len(t, patches, 1)
		assert.Equal(t, map[string]string{"status": "shipped", "tracking_number": "20450000000000"}, patches[0])
		assert.Equal(t, modeList, m.mode)
	})

	main.Run("removed shortcuts do nothing", func(t *testing.T) {
		var patches []map[string]string
		m := start(t, &patches)
		for _, k := range []string{"p", "t", "d", "c"} {
			m.Update(key(k))
			assert.Equal(t, modeList, m.mode, k)
		}
		assert.Empty(t, patches)
	})

	main.Run("picker offers only allowed targets", func(t *testing.T) {
		assert.Equal(t, []string{"processing", "refund_requested", "refunded", "cancelled"}, allowedTargets("paid"))
		assert.Equal(t, []string{"cancelled"}, allowedTargets("awaiting_payment"))
		assert.Empty(t, allowedTargets("cancelled"))
		assert.Empty(t, allowedTargets("refunded"))
	})

}
