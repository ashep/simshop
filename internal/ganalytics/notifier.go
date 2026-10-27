package ganalytics

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/ashep/simshop/internal/order"
)

const (
	bufferSize  = 256
	maxRetries  = 3
	stopTimeout = 5 * time.Second
	sendTimeout = 5 * time.Second
)

// backoffSchedule holds the sleeps between attempts 1→2 and 2→3.
var backoffSchedule = []time.Duration{1 * time.Second, 2 * time.Second}

// eventByStatus maps the order statuses that matter to GA to the GA4 event
// they produce. Every other status is ignored.
var eventByStatus = map[string]string{
	"paid":     "purchase",
	"refunded": "refund",
}

// eventSender is the slice of *Client that Notifier needs.
type eventSender interface {
	Send(ctx context.Context, p Payload) error
}

// productLookup resolves a product id to its display title; "" when unknown.
type productLookup interface {
	Title(id string) string
}

// Notifier reports paid and refunded orders to Google Analytics. It implements
// order.Notifier: Notify is non-blocking and a single background worker reads
// the order and posts the event with bounded retry. Call Start before
// producers emit events and Stop on shutdown.
type Notifier struct {
	client   eventSender
	reader   order.Reader
	products productLookup
	log      zerolog.Logger
	events   chan order.NotificationEvent
	done     chan struct{}
	once     sync.Once
	stopped  sync.Once
	sleepFn  func(time.Duration)
}

// NewNotifier returns a Notifier.
func NewNotifier(client eventSender, reader order.Reader, products productLookup, log zerolog.Logger) *Notifier {
	return &Notifier{
		client:   client,
		reader:   reader,
		products: products,
		log:      log,
		events:   make(chan order.NotificationEvent, bufferSize),
		done:     make(chan struct{}),
		sleepFn:  time.Sleep,
	}
}

// Notify enqueues evt when its status maps to a GA event. Non-blocking: a full
// buffer or a stopped notifier drops the event with a Warn log.
func (n *Notifier) Notify(_ context.Context, evt order.NotificationEvent) {
	if _, ok := eventByStatus[evt.Status]; !ok {
		return
	}
	defer func() {
		// Sending on a closed channel panics; a producer racing with Stop drops instead.
		if r := recover(); r != nil {
			n.log.Warn().Str("order_id", evt.OrderID).Str("order_status", evt.Status).
				Msg("google analytics notifier closed, dropping event")
		}
	}()
	select {
	case n.events <- evt:
	default:
		n.log.Warn().Str("order_id", evt.OrderID).Str("order_status", evt.Status).
			Msg("google analytics notifier buffer full, dropping event")
	}
}

// Start launches the worker goroutine. Subsequent calls are no-ops.
func (n *Notifier) Start() {
	n.once.Do(func() {
		go n.run()
	})
}

// Stop closes the queue and waits up to 5s for the worker to drain it.
func (n *Notifier) Stop() {
	n.stopped.Do(func() {
		close(n.events)
		select {
		case <-n.done:
		case <-time.After(stopTimeout):
			n.log.Warn().Msg("google analytics notifier shutdown drain timed out")
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
		n.log.Warn().Str("order_id", evt.OrderID).Str("order_status", evt.Status).
			Msg("google analytics notifier: order not found")
		return
	}
	if err != nil {
		n.log.Error().Err(err).Str("order_id", evt.OrderID).Str("order_status", evt.Status).
			Msg("google analytics notifier: reader.GetByID failed")
		return
	}

	n.sendWithRetry(ctx, evt, buildPayload(rec, eventByStatus[evt.Status], n.products))
}

func (n *Notifier) sendWithRetry(ctx context.Context, evt order.NotificationEvent, p Payload) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
		err := n.client.Send(sendCtx, p)
		cancel()
		if err == nil {
			n.log.Info().Str("order_id", evt.OrderID).Str("event", p.Events[0].Name).
				Msg("google analytics event sent")
			return
		}

		var apiErr *APIError
		permanent := errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 &&
			apiErr.HTTPStatus != http.StatusTooManyRequests
		n.log.Warn().Err(err).Str("order_id", evt.OrderID).Str("order_status", evt.Status).
			Int("attempt", attempt).Msg("google analytics notifier: send failed")
		if permanent || attempt == maxRetries {
			n.log.Error().Err(err).Str("order_id", evt.OrderID).Str("order_status", evt.Status).
				Msg("google analytics notifier: dropping event")
			return
		}
		n.sleepFn(backoffSchedule[attempt-1])
	}
}

// buildPayload renders the GA4 ecommerce event for rec. An order placed without
// analytics ids still reports revenue: the order id stands in for client_id, so
// the sale is counted but not attributed to a traffic source.
func buildPayload(rec *order.Record, name string, products productLookup) Payload {
	value := float64(rec.Price) / 100
	title := products.Title(rec.ProductID)
	if title == "" {
		title = rec.ProductID
	}
	variants := make([]string, 0, len(rec.Attrs))
	for _, a := range rec.Attrs {
		variants = append(variants, a.Value)
	}

	params := map[string]any{
		"transaction_id": rec.ID,
		"value":          value,
		"currency":       strings.ToUpper(rec.Currency),
		"items": []Item{{
			ItemID:      rec.ProductID,
			ItemName:    title,
			ItemVariant: strings.Join(variants, ", "),
			Price:       value,
			Quantity:    1,
		}},
	}
	if rec.Promo != nil {
		params["coupon"] = rec.Promo.Code
	}
	if rec.GASessionID != "" {
		// GA ties an event to a session only when both are present.
		params["session_id"] = rec.GASessionID
		params["engagement_time_msec"] = 1
	}

	clientID := rec.GAClientID
	if clientID == "" {
		clientID = rec.ID
	}
	return Payload{ClientID: clientID, Events: []Event{{Name: name, Params: params}}}
}
