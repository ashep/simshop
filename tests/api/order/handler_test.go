//go:build functest

package order_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type recordedTGRequest struct {
	Path      string
	ChatID    string
	Text      string
	ParseMode string
}

// newTelegramStub returns an httptest.Server that records every POST onto a
// buffered channel and replies with a canned ok=true Telegram response. The
// production code only calls /bot<token>/sendMessage; any other path is also
// recorded so a regression that hits a different URL fails loudly.
func newTelegramStub(t *testing.T) (*httptest.Server, <-chan recordedTGRequest) {
	t.Helper()
	ch := make(chan recordedTGRequest, 32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var parsed struct {
			ChatID    string `json:"chat_id"`
			Text      string `json:"text"`
			ParseMode string `json:"parse_mode"`
		}
		_ = json.Unmarshal(body, &parsed)
		select {
		case ch <- recordedTGRequest{Path: r.URL.Path, ChatID: parsed.ChatID, Text: parsed.Text, ParseMode: parsed.ParseMode}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

// waitForTGRequest polls ch with a deadline; returns the next recorded
// request or fails the test if none arrives in time.
func waitForTGRequest(t *testing.T, ch <-chan recordedTGRequest, deadline time.Duration) recordedTGRequest {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(deadline):
		t.Fatalf("expected a Telegram request within %s, got none", deadline)
		return recordedTGRequest{}
	}
}

// drainTGChannel pulls every immediately-available recorded request, leaving
// the channel empty. Use this between subtests that share a stub.
func drainTGChannel(ch <-chan recordedTGRequest) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
