package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/ashep/simshop/internal/order"
)

const (
	bufferSize    = 256
	maxRetries    = 3
	stopTimeout   = 5 * time.Second
	maxRetryAfter = 30 * time.Second
	minRetryAfter = 1 * time.Second
	sendTimeout   = 5 * time.Second
)

// backoffSchedule holds the inter-attempt sleep durations. With maxRetries=3,
// there are only two sleeps (between attempts 1→2 and 2→3); attempt 3 either
// succeeds or is dropped without sleeping.
var backoffSchedule = []time.Duration{1 * time.Second, 2 * time.Second}

// messageSender is the slice of *Client that Notifier needs. Defined as an
// interface so tests can substitute a recording fake without an httptest
// server.
type messageSender interface {
	SendMessage(ctx context.Context, chatID, text, parseMode string) error
}

// productLookup resolves a product id to a display title (typically rendered
// from products.yaml). The notifier uses it for the *Product:* line in the
// new-order message. When the lookup returns "", the notifier falls back to
// the raw product id.
type productLookup interface {
	Title(id string) string
}

// parseMode is the Telegram parse_mode used for all notifier messages.
// MarkdownV2 supports `inline code` (used for the order id, so it's
// tap-to-copy in the chat client) and *bold* (used for the status).
const parseMode = "MarkdownV2"

// Notifier mirrors order_history inserts to a Telegram chat. It implements
// order.Notifier; producers call Notify (non-blocking) and a single background
// goroutine drains the events channel, formats messages, and posts to
// Telegram with bounded retry. Construct with NewNotifier; call Start to begin
// the worker; call Stop to drain buffered events and shut down (blocks up to
// 5s).
type Notifier struct {
	client   messageSender
	chatID   string
	reader   order.Reader
	products productLookup
	log      zerolog.Logger
	events   chan order.NotificationEvent
	done     chan struct{}
	once     sync.Once
	stopped  sync.Once
	sleepFn  func(time.Duration)
}

// NewNotifier returns a Notifier. The caller must call Start before producers
// emit events and Stop on shutdown.
func NewNotifier(client messageSender, chatID string, reader order.Reader, products productLookup, log zerolog.Logger) *Notifier {
	return &Notifier{
		client:   client,
		chatID:   chatID,
		reader:   reader,
		products: products,
		log:      log,
		events:   make(chan order.NotificationEvent, bufferSize),
		done:     make(chan struct{}),
		sleepFn:  time.Sleep,
	}
}

// Notify enqueues evt for asynchronous delivery. Non-blocking: if the buffer
// is full or the notifier has been stopped, the event is dropped with a Warn
// log and Notify returns immediately.
func (n *Notifier) Notify(ctx context.Context, evt order.NotificationEvent) {
	defer func() {
		// Sending on a closed channel panics; recover so a producer racing
		// with Stop drops the event instead of crashing.
		if r := recover(); r != nil {
			n.log.Warn().
				Str("order_id", evt.OrderID).
				Str("order_status", evt.Status).
				Msg("telegram notifier closed, dropping event")
		}
	}()
	select {
	case n.events <- evt:
	default:
		n.log.Warn().
			Str("order_id", evt.OrderID).
			Str("order_status", evt.Status).
			Msg("telegram notifier buffer full, dropping event")
	}
}

// Start launches the worker goroutine. Safe to call once. Subsequent calls
// are no-ops.
func (n *Notifier) Start() {
	n.once.Do(func() {
		go n.run()
	})
}

// Stop closes the events channel and waits up to 5s for the worker to drain
// remaining events. After Stop returns, further Notify calls drop their
// events.
func (n *Notifier) Stop() {
	n.stopped.Do(func() {
		close(n.events)
		select {
		case <-n.done:
		case <-time.After(stopTimeout):
			n.log.Warn().Msg("telegram notifier shutdown drain timed out")
		}
	})
}

func (n *Notifier) run() {
	defer close(n.done)
	for evt := range n.events {
		n.handle(evt)
	}
}

func (n *Notifier) handle(evt order.NotificationEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout*maxRetries+5*time.Second)
	defer cancel()

	rec, err := n.reader.GetByID(ctx, evt.OrderID)
	if errors.Is(err, order.ErrNotFound) {
		n.log.Warn().
			Str("order_id", evt.OrderID).
			Str("order_status", evt.Status).
			Msg("telegram notifier: order not found")
		return
	}
	if err != nil {
		n.log.Error().
			Err(err).
			Str("order_id", evt.OrderID).
			Str("order_status", evt.Status).
			Msg("telegram notifier: reader.GetByID failed")
		return
	}

	text := formatMessage(rec, evt.Status, evt.Note, evt.TrackingNumber, n.products)
	n.sendWithRetry(ctx, evt, text)
}

func (n *Notifier) sendWithRetry(ctx context.Context, evt order.NotificationEvent, text string) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
		err := n.client.SendMessage(sendCtx, n.chatID, text, parseMode)
		cancel()
		if err == nil {
			return
		}

		var apiErr *APIError
		isAPI := errors.As(err, &apiErr)
		permanent := isAPI && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 && apiErr.HTTPStatus != http.StatusTooManyRequests

		level := n.log.Warn()
		level.Err(err).
			Str("order_id", evt.OrderID).
			Str("order_status", evt.Status).
			Str("chat_id", n.chatID).
			Int("attempt", attempt)
		if isAPI {
			level = level.Int("last_status", apiErr.HTTPStatus)
		}
		level.Msg("telegram notifier: send failed")

		if permanent {
			n.log.Error().
				Err(err).
				Str("order_id", evt.OrderID).
				Str("order_status", evt.Status).
				Int("last_status", apiErr.HTTPStatus).
				Msg("telegram notifier: permanent error, dropping")
			return
		}

		if attempt == maxRetries {
			n.log.Error().
				Err(err).
				Str("order_id", evt.OrderID).
				Str("order_status", evt.Status).
				Int("attempts", attempt).
				Msg("telegram notifier: retry budget exhausted, dropping")
			return
		}

		sleep := backoffSchedule[attempt-1]
		if isAPI && apiErr.RetryAfter > 0 {
			sleep = apiErr.RetryAfter
			if sleep < minRetryAfter {
				sleep = minRetryAfter
			}
			if sleep > maxRetryAfter {
				sleep = maxRetryAfter
			}
		}
		n.sleepFn(sleep)
	}
}

// mdv2Escaper escapes the reserved MarkdownV2 characters listed in
// https://core.telegram.org/bots/api#markdownv2-style. Used for any text
// rendered outside of inline-code spans (UUIDs in code spans don't need
// escaping because they contain none of MarkdownV2's reserved characters).
var mdv2Escaper = strings.NewReplacer(
	`\`, `\\`,
	"_", `\_`, "*", `\*`, "[", `\[`, "]", `\]`,
	"(", `\(`, ")", `\)`, "~", `\~`, "`", "\\`",
	">", `\>`, "#", `\#`, "+", `\+`, "-", `\-`,
	"=", `\=`, "|", `\|`, "{", `\{`, "}", `\}`,
	".", `\.`, "!", `\!`,
)

func mdv2Escape(s string) string { return mdv2Escaper.Replace(s) }

// formatMessage renders the MarkdownV2 Telegram message for one event.
//
// On status="new" the message includes full order detail; on every other
// status it is the slim form (id, status, optional tracking number, optional
// status note) — see the Telegram-notifications section in README.md.
func formatMessage(rec *order.Record, status, statusNote, trackingNumber string, products productLookup) string {
	if status == "new" {
		return formatNewOrder(rec, statusNote, products)
	}
	return formatStatusUpdate(rec.ID, status, statusNote, trackingNumber)
}

func formatNewOrder(rec *order.Record, statusNote string, products productLookup) string {
	var b strings.Builder
	// Header: order id in inline-code so it's tap-to-copy in the Telegram
	// client. UUIDs contain no MarkdownV2-reserved characters, so the id is
	// inserted raw between backticks.
	fmt.Fprintf(&b, "*New order* `%s`\n\n", rec.ID)
	productLabel := rec.ProductID
	if products != nil {
		if title := products.Title(rec.ProductID); title != "" {
			productLabel = title
		}
	}
	fmt.Fprintf(&b, "*Product:* %s\n", mdv2Escape(productLabel))
	for _, a := range rec.Attrs {
		fmt.Fprintf(&b, "*%s:* %s\n", mdv2Escape(a.Name), mdv2Escape(a.Value))
	}
	totalText := fmt.Sprintf("%d.%02d %s", rec.Price/100, rec.Price%100, strings.ToUpper(rec.Currency))
	fmt.Fprintf(&b, "*Total:* %s\n", mdv2Escape(totalText))

	name := rec.FirstName
	if rec.MiddleName != nil && *rec.MiddleName != "" {
		name += " " + *rec.MiddleName
	}
	name += " " + rec.LastName
	fmt.Fprintf(&b, "*Customer:* %s\n", mdv2Escape(name))
	fmt.Fprintf(&b, "*Phone:* %s\n", mdv2Escape(rec.Phone))
	fmt.Fprintf(&b, "*Email:* %s\n", mdv2Escape(rec.Email))
	deliveryText := fmt.Sprintf("%s, %s, %s", strings.ToUpper(rec.Country), rec.City, rec.Address)
	fmt.Fprintf(&b, "*Delivery:* %s\n", mdv2Escape(deliveryText))

	if rec.CustomerNote != nil && *rec.CustomerNote != "" {
		fmt.Fprintf(&b, "*Customer note:* %s\n", mdv2Escape(*rec.CustomerNote))
	}
	if statusNote != "" {
		fmt.Fprintf(&b, "*Status note:* %s\n", mdv2Escape(statusNote))
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatStatusUpdate(orderID, status, statusNote, trackingNumber string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Order `%s` — *%s*", orderID, mdv2Escape(status))
	if trackingNumber != "" {
		fmt.Fprintf(&b, "\n\nTracking: `%s`", trackingNumber)
	}
	if statusNote != "" {
		fmt.Fprintf(&b, "\n\n%s", mdv2Escape(statusNote))
	}
	return b.String()
}
