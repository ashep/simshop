package metacapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

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

// eventSender is the slice of *Client that Notifier needs.
type eventSender interface {
	Send(ctx context.Context, events []Event) error
}

// Notifier reports paid orders to the Meta Conversions API as `Purchase`. It
// implements order.Notifier: Notify is non-blocking and a single background
// worker reads the order and posts the event with bounded retry. Call Start
// before producers emit events and Stop on shutdown.
type Notifier struct {
	client  eventSender
	reader  order.Reader
	log     zerolog.Logger
	events  chan order.NotificationEvent
	done    chan struct{}
	once    sync.Once
	stopped sync.Once
	sleepFn func(time.Duration)
	nowFn   func() time.Time
}

// NewNotifier returns a Notifier.
func NewNotifier(client eventSender, reader order.Reader, log zerolog.Logger) *Notifier {
	return &Notifier{
		client:  client,
		reader:  reader,
		log:     log,
		events:  make(chan order.NotificationEvent, bufferSize),
		done:    make(chan struct{}),
		sleepFn: time.Sleep,
		nowFn:   time.Now,
	}
}

// Notify enqueues paid-order events; every other status is ignored (Meta has no
// refund event). Non-blocking: a full buffer or a stopped notifier drops the
// event with a Warn log.
func (n *Notifier) Notify(_ context.Context, evt order.NotificationEvent) {
	if evt.Status != "paid" {
		return
	}
	defer func() {
		// Sending on a closed channel panics; a producer racing with Stop drops instead.
		if r := recover(); r != nil {
			n.log.Warn().Str("order_id", evt.OrderID).Msg("meta notifier closed, dropping event")
		}
	}()
	select {
	case n.events <- evt:
	default:
		n.log.Warn().Str("order_id", evt.OrderID).Msg("meta notifier buffer full, dropping event")
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
			n.log.Warn().Msg("meta notifier shutdown drain timed out")
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
		n.log.Warn().Str("order_id", evt.OrderID).Msg("meta notifier: order not found")
		return
	}
	if err != nil {
		n.log.Error().Err(err).Str("order_id", evt.OrderID).Msg("meta notifier: reader.GetByID failed")
		return
	}

	n.sendWithRetry(ctx, evt, []Event{buildPurchase(rec, n.nowFn())})
}

func (n *Notifier) sendWithRetry(ctx context.Context, evt order.NotificationEvent, events []Event) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
		err := n.client.Send(sendCtx, events)
		cancel()
		if err == nil {
			n.log.Info().Str("order_id", evt.OrderID).Msg("meta purchase event sent")
			return
		}

		var apiErr *APIError
		permanent := errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 &&
			apiErr.HTTPStatus != http.StatusTooManyRequests
		n.log.Warn().Err(err).Str("order_id", evt.OrderID).Int("attempt", attempt).
			Msg("meta notifier: send failed")
		if permanent || attempt == maxRetries {
			n.log.Error().Err(err).Str("order_id", evt.OrderID).Msg("meta notifier: dropping event")
			return
		}
		n.sleepFn(backoffSchedule[attempt-1])
	}
}

// buildPurchase renders the Purchase event for rec. event_id is the order id, so
// a browser-side Purchase with the same id (should one ever be added) dedupes.
func buildPurchase(rec *order.Record, now time.Time) Event {
	phone := digitsOnly(rec.Phone)
	return Event{
		EventName:    "Purchase",
		EventTime:    now.Unix(),
		EventID:      rec.ID,
		ActionSource: "website",
		UserData: UserData{
			Email:           hashed(strings.ToLower(strings.TrimSpace(rec.Email))),
			Phone:           hashed(phone),
			FirstName:       hashed(strings.ToLower(strings.TrimSpace(rec.FirstName))),
			LastName:        hashed(strings.ToLower(strings.TrimSpace(rec.LastName))),
			City:            hashed(lettersOnly(strings.ToLower(rec.City))),
			Country:         hashed(strings.ToLower(strings.TrimSpace(rec.Country))),
			ExternalID:      hashed(phone), // the shop's own customer key: one promo per phone
			FBP:             rec.MetaFBP,
			FBC:             rec.MetaFBC,
			ClientIPAddress: rec.ClientIP,
			ClientUserAgent: rec.UserAgent,
		},
		CustomData: CustomData{
			Currency:    strings.ToUpper(rec.Currency),
			Value:       float64(rec.Price) / 100,
			ContentIDs:  []string{rec.ProductID},
			ContentType: "product",
			OrderID:     rec.ID,
			NumItems:    1,
		},
	}
}

// hashed returns the SHA-256 hex of an already-normalized value as a one-element
// list, or nil for an empty value so the key is omitted.
func hashed(v string) []string {
	if v == "" {
		return nil
	}
	h := sha256.Sum256([]byte(v))
	return []string{hex.EncodeToString(h[:])}
}

func digitsOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// lettersOnly drops spaces and punctuation, as Meta's city normalization asks.
func lettersOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) {
			return r
		}
		return -1
	}, s)
}
