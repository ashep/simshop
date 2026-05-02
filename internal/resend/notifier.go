package resend

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

// backoffSchedule holds inter-attempt sleeps. With maxRetries=3, only two
// sleeps fire (between 1→2 and 2→3); attempt 3 either succeeds or is dropped
// without sleeping.
var backoffSchedule = []time.Duration{1 * time.Second, 2 * time.Second}

// notifyStatuses is the customer-email allow-set. Any status not in this set
// is silently dropped before any DB read or template lookup. Statuses that
// drive operator-only notifications (e.g. "new") deliberately don't reach the
// customer.
var notifyStatuses = map[string]bool{
	"paid":             true,
	"shipped":          true,
	"delivered":        true,
	"refund_requested": true,
	"refunded":         true,
}

// emailSender is the slice of *Client that Notifier needs. Defined as an
// interface so tests can substitute a recording fake without an httptest
// server.
type emailSender interface {
	SendEmail(ctx context.Context, e Email) error
}

// productLookup resolves a product id to a display title. The notifier uses
// it for the rendered TemplateData.ProductTitle. When the lookup returns "",
// the notifier falls back to the raw product id.
type productLookup interface {
	Title(id string) string
}

// shopLookup resolves a shop display name for the given language. Falls back
// to "en" when the requested language is missing; ultimately to the empty
// string if neither exists.
type shopLookup interface {
	Name(lang string) string
}

// templateStore renders a (status, lang) pair into (subject, html, text).
// Implemented by *TemplateStore in templates.go; defined as an interface so
// tests can substitute a recording fake.
type templateStore interface {
	Render(status, lang string, data TemplateData) (subject, html, text string, err error)
}

// Notifier dispatches customer-facing status-change emails via Resend. It
// implements order.Notifier; producers call Notify (non-blocking) and a
// single background worker drains the events channel, looks up the order,
// renders the templated email, and sends via the Resend API with bounded
// retry. Construct with NewNotifier; call Start to begin the worker; call
// Stop to drain buffered events and shut down (blocks up to 5s).
type Notifier struct {
	client    emailSender
	from      string
	orderURL  string
	reader    order.Reader
	products  productLookup
	shop      shopLookup
	templates templateStore
	log       zerolog.Logger
	events    chan order.NotificationEvent
	done      chan struct{}
	once      sync.Once
	stopped   sync.Once
	sleepFn   func(time.Duration)
}

// NewNotifier returns a Notifier. Caller must Start before producing events
// and Stop on shutdown. orderURL is a template containing the literal
// substring "{id}" which is replaced with the order id when rendering
// TemplateData.OrderURL.
func NewNotifier(
	client emailSender,
	from string,
	orderURL string,
	reader order.Reader,
	products productLookup,
	shop shopLookup,
	templates templateStore,
	log zerolog.Logger,
) *Notifier {
	return &Notifier{
		client:    client,
		from:      from,
		orderURL:  orderURL,
		reader:    reader,
		products:  products,
		shop:      shop,
		templates: templates,
		log:       log,
		events:    make(chan order.NotificationEvent, bufferSize),
		done:      make(chan struct{}),
		sleepFn:   time.Sleep,
	}
}

// Notify enqueues evt for asynchronous delivery. Non-blocking: if the buffer
// is full or the notifier has been stopped, the event is dropped with a Warn
// log and Notify returns immediately.
func (n *Notifier) Notify(ctx context.Context, evt order.NotificationEvent) {
	_ = ctx // worker creates its own context; caller's deadline doesn't apply.
	defer func() {
		// Sending on a closed channel panics; recover so a producer racing
		// with Stop drops the event instead of crashing.
		if r := recover(); r != nil {
			n.log.Warn().
				Str("order_id", evt.OrderID).
				Str("order_status", evt.Status).
				Msg("resend notifier closed, dropping event")
		}
	}()
	select {
	case n.events <- evt:
	default:
		n.log.Warn().
			Str("order_id", evt.OrderID).
			Str("order_status", evt.Status).
			Msg("resend notifier buffer full, dropping event")
	}
}

// Start launches the worker goroutine. Safe to call once; subsequent calls
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
			n.log.Warn().Msg("resend notifier shutdown drain timed out")
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
	if !notifyStatuses[evt.Status] {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout*maxRetries+5*time.Second)
	defer cancel()

	rec, err := n.reader.GetByID(ctx, evt.OrderID)
	if errors.Is(err, order.ErrNotFound) {
		n.log.Warn().
			Str("order_id", evt.OrderID).
			Str("order_status", evt.Status).
			Msg("resend notifier: order not found")
		return
	}
	if err != nil {
		n.log.Error().Err(err).
			Str("order_id", evt.OrderID).
			Str("order_status", evt.Status).
			Msg("resend notifier: reader.GetByID failed")
		return
	}

	data := n.buildData(rec, evt)
	subject, html, text, err := n.templates.Render(evt.Status, rec.Lang, data)
	if err != nil {
		n.log.Error().Err(err).
			Str("order_id", evt.OrderID).
			Str("order_status", evt.Status).
			Str("template_lang", rec.Lang).
			Msg("resend notifier: template render failed")
		return
	}

	n.sendWithRetry(ctx, evt, Email{
		From: n.from, To: rec.Email, Subject: subject, HTML: html, Text: text,
	})
}

func (n *Notifier) buildData(rec *order.Record, evt order.NotificationEvent) TemplateData {
	short := rec.ID
	if len(short) > 13 {
		short = short[:13]
	}
	name := rec.FirstName
	if rec.MiddleName != nil && *rec.MiddleName != "" {
		name += " " + *rec.MiddleName
	}
	name += " " + rec.LastName

	productTitle := rec.ProductID
	if t := n.products.Title(rec.ProductID); t != "" {
		productTitle = t
	}

	// shopLookup.Name is expected to apply its own lang-fallback (the
	// production *shop.Service falls back to the alphabetically-first
	// language when the requested one is missing).
	shopName := n.shop.Name(rec.Lang)

	total := fmt.Sprintf("%d.%02d %s", rec.Price/100, rec.Price%100, strings.ToUpper(rec.Currency))

	orderURL := strings.ReplaceAll(n.orderURL, "{id}", rec.ID)

	return TemplateData{
		OrderID:      rec.ID,
		OrderShortID: short,
		CustomerName: name,
		ProductTitle: productTitle,
		Attrs:        rec.Attrs,
		Total:        total,
		StatusNote:   evt.Note,
		ShopName:     shopName,
		OrderURL:     orderURL,
	}
}

func (n *Notifier) sendWithRetry(ctx context.Context, evt order.NotificationEvent, e Email) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
		err := n.client.SendEmail(sendCtx, e)
		cancel()
		if err == nil {
			return
		}

		var apiErr *APIError
		isAPI := errors.As(err, &apiErr)
		permanent := isAPI && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 && apiErr.HTTPStatus != http.StatusTooManyRequests

		ev := n.log.Warn().Err(err).
			Str("order_id", evt.OrderID).
			Str("order_status", evt.Status).
			Str("to", e.To).
			Int("attempt", attempt)
		if isAPI {
			ev = ev.Int("last_status", apiErr.HTTPStatus)
		}
		ev.Msg("resend notifier: send failed")

		if permanent {
			n.log.Error().Err(err).
				Str("order_id", evt.OrderID).
				Str("order_status", evt.Status).
				Int("last_status", apiErr.HTTPStatus).
				Msg("resend notifier: permanent error, dropping")
			return
		}

		if attempt == maxRetries {
			n.log.Error().Err(err).
				Str("order_id", evt.OrderID).
				Str("order_status", evt.Status).
				Int("attempts", attempt).
				Msg("resend notifier: retry budget exhausted, dropping")
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
