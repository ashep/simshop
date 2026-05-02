package telegram

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/order"
)

type fakeReader struct {
	mu      sync.Mutex
	records map[string]*order.Record
	err     error
}

func newFakeReader() *fakeReader {
	return &fakeReader{records: map[string]*order.Record{}}
}

func (r *fakeReader) put(rec *order.Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[rec.ID] = rec
}

func (r *fakeReader) List(ctx context.Context) ([]order.Record, error) {
	return nil, errors.New("not used")
}

func (r *fakeReader) GetByID(ctx context.Context, id string) (*order.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	rec, ok := r.records[id]
	if !ok {
		return nil, order.ErrNotFound
	}
	return rec, nil
}

func (r *fakeReader) GetStatus(ctx context.Context, id string) (string, error) {
	return "", errors.New("not used")
}

type fakeSender struct {
	mu      sync.Mutex
	calls   []sentMessage
	errs    []error // pop front per call; nil-tail = success after exhaustion
	blockCh chan struct{}
}

type sentMessage struct {
	chatID    string
	text      string
	parseMode string
	at        time.Time
}

func (s *fakeSender) SendMessage(ctx context.Context, chatID, text, parseMode string) error {
	if s.blockCh != nil {
		select {
		case <-s.blockCh:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, sentMessage{chatID: chatID, text: text, parseMode: parseMode, at: time.Now()})
	if len(s.errs) == 0 {
		return nil
	}
	e := s.errs[0]
	s.errs = s.errs[1:]
	return e
}

func (s *fakeSender) snapshot() []sentMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]sentMessage, len(s.calls))
	copy(out, s.calls)
	return out
}

func sampleRecord() *order.Record {
	return &order.Record{
		ID:        "018f4e3a-0000-7000-8000-000000000001",
		ProductID: "pro-plan-annual",
		Status:    "new",
		Email:     "jane@example.com",
		Price:     4900,
		Currency:  "USD",
		FirstName: "Jane",
		LastName:  "Doe",
		Phone:     "+1234567890",
		Country:   "ua",
		City:      "Kyiv",
		Address:   "Some Street, 5",
		Attrs:     []order.Attr{},
		History:   []order.HistoryEntry{},
		Invoices:  []order.Invoice{},
	}
}

func waitForCalls(t *testing.T, s *fakeSender, want int, deadline time.Duration) []sentMessage {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		got := s.snapshot()
		if len(got) >= want {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := s.snapshot()
	t.Fatalf("expected %d sender calls within %s, got %d", want, deadline, len(got))
	return got
}

// fakeProductLookup is a stub productLookup whose Title returns the configured
// map entry (or "" when missing). The default-zero value (empty map) makes
// Title always return "" — exercising the notifier's fallback to the raw id.
type fakeProductLookup struct{ titles map[string]string }

func (f fakeProductLookup) Title(id string) string { return f.titles[id] }

func newTestNotifier(t *testing.T, sender messageSender, reader order.Reader) *Notifier {
	t.Helper()
	return newTestNotifierWithProducts(t, sender, reader, fakeProductLookup{})
}

func newTestNotifierWithProducts(t *testing.T, sender messageSender, reader order.Reader, products productLookup) *Notifier {
	t.Helper()
	n := NewNotifier(sender, "@chan", reader, products, zerolog.Nop())
	n.sleepFn = func(time.Duration) {} // no real sleeps in tests
	t.Cleanup(n.Stop)
	n.Start()
	return n
}

func TestNotifier(main *testing.T) {
	main.Run("NewOrderHappyPath", func(t *testing.T) {
		r := newFakeReader()
		r.put(sampleRecord())
		s := &fakeSender{}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: "018f4e3a-0000-7000-8000-000000000001",
			Status:  "new",
		})

		got := waitForCalls(t, s, 1, time.Second)
		assert.Equal(t, "@chan", got[0].chatID)
		assert.Equal(t, "MarkdownV2", got[0].parseMode)
		assert.Contains(t, got[0].text, "*New order* `018f4e3a-0000-7000-8000-000000000001`")
		assert.Contains(t, got[0].text, `*Product:* pro\-plan\-annual`)
		assert.Contains(t, got[0].text, `*Total:* 49\.00 USD`)
		assert.Contains(t, got[0].text, `*Customer:* Jane Doe`)
		assert.Contains(t, got[0].text, `*Phone:* \+1234567890`)
		assert.Contains(t, got[0].text, `*Email:* jane@example\.com`)
		assert.Contains(t, got[0].text, `*Delivery:* UA, Kyiv, Some Street, 5`)
		assert.NotContains(t, got[0].text, "Customer note:")
		assert.NotContains(t, got[0].text, "Status note:")
	})

	main.Run("NewOrderUsesProductTitleWhenLookupResolves", func(t *testing.T) {
		r := newFakeReader()
		r.put(sampleRecord())
		s := &fakeSender{}
		products := fakeProductLookup{titles: map[string]string{"pro-plan-annual": "Pro Plan (Annual)"}}
		n := newTestNotifierWithProducts(t, s, r, products)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: "018f4e3a-0000-7000-8000-000000000001",
			Status:  "new",
		})

		got := waitForCalls(t, s, 1, time.Second)
		assert.Contains(t, got[0].text, `*Product:* Pro Plan \(Annual\)`)
		assert.NotContains(t, got[0].text, "*Product:* pro")
	})

	main.Run("NewOrderIncludesMiddleName", func(t *testing.T) {
		r := newFakeReader()
		rec := sampleRecord()
		mid := "Q."
		rec.MiddleName = &mid
		r.put(rec)
		s := &fakeSender{}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{OrderID: rec.ID, Status: "new"})

		got := waitForCalls(t, s, 1, time.Second)
		assert.Contains(t, got[0].text, `*Customer:* Jane Q\. Doe`)
	})

	main.Run("NewOrderAttrsRendered", func(t *testing.T) {
		r := newFakeReader()
		rec := sampleRecord()
		rec.Attrs = []order.Attr{
			{Name: "Display color", Value: "Red", Price: 0},
			{Name: "Material", Value: "Aluminum", Price: 500},
		}
		r.put(rec)
		s := &fakeSender{}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: rec.ID,
			Status:  "new",
		})

		got := waitForCalls(t, s, 1, time.Second)
		assert.Contains(t, got[0].text, "*Display color:* Red")
		assert.Contains(t, got[0].text, "*Material:* Aluminum")
		// Attrs appear between Product and Total lines, in persisted order.
		productIdx := strings.Index(got[0].text, "*Product:*")
		colorIdx := strings.Index(got[0].text, "*Display color:*")
		materialIdx := strings.Index(got[0].text, "*Material:*")
		totalIdx := strings.Index(got[0].text, "*Total:*")
		assert.True(t, productIdx < colorIdx && colorIdx < materialIdx && materialIdx < totalIdx,
			"attrs must appear in persisted order between Product and Total")
	})

	main.Run("NewOrderIncludesCustomerNote", func(t *testing.T) {
		r := newFakeReader()
		rec := sampleRecord()
		note := "Please ship after Friday"
		rec.CustomerNote = &note
		r.put(rec)
		s := &fakeSender{}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: rec.ID,
			Status:  "new",
		})

		got := waitForCalls(t, s, 1, time.Second)
		assert.Contains(t, got[0].text, "*Customer note:* Please ship after Friday")
	})

	main.Run("StatusUpdatePaidIsSlim", func(t *testing.T) {
		r := newFakeReader()
		r.put(sampleRecord())
		s := &fakeSender{}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: "018f4e3a-0000-7000-8000-000000000001",
			Status:  "paid",
			Note:    "monobank: success, finalAmount=4900",
		})

		got := waitForCalls(t, s, 1, time.Second)
		assert.Equal(t, "MarkdownV2", got[0].parseMode)
		assert.Contains(t, got[0].text, "Order `018f4e3a-0000-7000-8000-000000000001` — *paid*")
		assert.Contains(t, got[0].text, `monobank: success, finalAmount\=4900`)
		// Slim format must not include any of the full-info fields.
		assert.NotContains(t, got[0].text, "Product:")
		assert.NotContains(t, got[0].text, "Total:")
		assert.NotContains(t, got[0].text, "Customer:")
		assert.NotContains(t, got[0].text, "Customer note:")
	})

	main.Run("StatusUpdateAwaitingPaymentNoNote", func(t *testing.T) {
		r := newFakeReader()
		r.put(sampleRecord())
		s := &fakeSender{}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: "018f4e3a-0000-7000-8000-000000000001",
			Status:  "awaiting_payment",
		})

		got := waitForCalls(t, s, 1, time.Second)
		assert.Equal(t, "Order `018f4e3a-0000-7000-8000-000000000001` — *awaiting\\_payment*", got[0].text)
	})

	main.Run("BufferFullDropsAndContinues", func(t *testing.T) {
		r := newFakeReader()
		r.put(sampleRecord())
		release := make(chan struct{})
		s := &fakeSender{blockCh: release}
		// Build the notifier WITHOUT calling Start so we can pre-fill the buffer.
		n := NewNotifier(s, "@chan", r, fakeProductLookup{}, zerolog.Nop())
		n.sleepFn = func(time.Duration) {}
		t.Cleanup(n.Stop)
		// Fill buffer to capacity.
		for i := 0; i < cap(n.events); i++ {
			n.Notify(context.Background(), order.NotificationEvent{
				OrderID: "018f4e3a-0000-7000-8000-000000000001",
				Status:  "new",
			})
		}
		// One more — must drop because no consumer is running yet.
		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: "018f4e3a-0000-7000-8000-000000000001",
			Status:  "new",
		})
		// Now drain.
		n.Start()
		close(release)
		got := waitForCalls(t, s, cap(n.events), 2*time.Second)
		assert.Equal(t, cap(n.events), len(got), "exactly one event was dropped")
	})

	main.Run("ReaderNotFoundIsLoggedAndDropped", func(t *testing.T) {
		r := newFakeReader() // no records
		s := &fakeSender{}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: "missing",
			Status:  "new",
		})

		time.Sleep(100 * time.Millisecond)
		assert.Empty(t, s.snapshot(), "sender must not be called when reader returns ErrNotFound")
	})

	main.Run("ReaderGenericErrorIsLoggedAndDropped", func(t *testing.T) {
		r := newFakeReader()
		r.err = errors.New("db down")
		s := &fakeSender{}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: "any",
			Status:  "new",
		})

		time.Sleep(100 * time.Millisecond)
		assert.Empty(t, s.snapshot(), "sender must not be called when reader returns generic error")
	})

	main.Run("RetryAfter429UsesAPIErrorRetryAfter", func(t *testing.T) {
		r := newFakeReader()
		r.put(sampleRecord())
		s := &fakeSender{
			errs: []error{
				&APIError{HTTPStatus: http.StatusTooManyRequests, RetryAfter: 2 * time.Second},
				nil,
			},
		}
		var sleeps []time.Duration
		var sleepMu sync.Mutex
		n := NewNotifier(s, "@chan", r, fakeProductLookup{}, zerolog.Nop())
		n.sleepFn = func(d time.Duration) {
			sleepMu.Lock()
			defer sleepMu.Unlock()
			sleeps = append(sleeps, d)
		}
		t.Cleanup(n.Stop)
		n.Start()

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: "018f4e3a-0000-7000-8000-000000000001",
			Status:  "new",
		})

		waitForCalls(t, s, 2, 2*time.Second)
		sleepMu.Lock()
		defer sleepMu.Unlock()
		require.GreaterOrEqual(t, len(sleeps), 1)
		assert.Equal(t, 2*time.Second, sleeps[0], "first sleep must equal RetryAfter")
	})

	main.Run("Transient5xxRetriedThreeTimesThenDropped", func(t *testing.T) {
		r := newFakeReader()
		r.put(sampleRecord())
		s := &fakeSender{
			errs: []error{
				&APIError{HTTPStatus: http.StatusBadGateway},
				&APIError{HTTPStatus: http.StatusBadGateway},
				&APIError{HTTPStatus: http.StatusBadGateway},
				nil, // would succeed if a 4th attempt fired (it must not)
			},
		}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: "018f4e3a-0000-7000-8000-000000000001",
			Status:  "new",
		})

		// Wait long enough for the 4th attempt window to definitively close.
		time.Sleep(200 * time.Millisecond)
		got := s.snapshot()
		assert.Equal(t, 3, len(got), "exactly 3 attempts on persistent 5xx")
	})

	main.Run("Permanent4xxNotRetried", func(t *testing.T) {
		r := newFakeReader()
		r.put(sampleRecord())
		s := &fakeSender{
			errs: []error{
				&APIError{HTTPStatus: http.StatusBadRequest, Description: "chat not found"},
				nil,
			},
		}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: "018f4e3a-0000-7000-8000-000000000001",
			Status:  "new",
		})

		time.Sleep(100 * time.Millisecond)
		got := s.snapshot()
		assert.Equal(t, 1, len(got), "no retries on 4xx")
	})

	main.Run("StopDrainsBufferedEvents", func(t *testing.T) {
		r := newFakeReader()
		r.put(sampleRecord())
		s := &fakeSender{}
		n := NewNotifier(s, "@chan", r, fakeProductLookup{}, zerolog.Nop())
		n.sleepFn = func(time.Duration) {}
		n.Start()

		const N = 10
		for i := 0; i < N; i++ {
			n.Notify(context.Background(), order.NotificationEvent{
				OrderID: "018f4e3a-0000-7000-8000-000000000001",
				Status:  "new",
			})
		}
		done := make(chan struct{})
		go func() { n.Stop(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Stop did not return within 5s")
		}
		assert.Equal(t, N, len(s.snapshot()), "all buffered events must be sent before Stop returns")
	})

	main.Run("NotifyAfterStopDoesNotPanic", func(t *testing.T) {
		// Defensive — if a producer races with shutdown we want to drop, not crash.
		r := newFakeReader()
		s := &fakeSender{}
		n := NewNotifier(s, "@chan", r, fakeProductLookup{}, zerolog.Nop())
		n.Start()
		n.Stop()

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Notify panicked after Stop: %v", r)
			}
		}()
		n.Notify(context.Background(), order.NotificationEvent{OrderID: "x", Status: "new"})
	})

	main.Run("RendersTrackingLineForShippedEvent", func(t *testing.T) {
		r := newFakeReader()
		rec := sampleRecord()
		rec.Status = "shipped"
		r.put(rec)
		s := &fakeSender{}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID:        rec.ID,
			Status:         "shipped",
			TrackingNumber: "TRK-XYZ",
		})

		got := waitForCalls(t, s, 1, 2*time.Second)
		assert.Contains(t, got[0].text, "Tracking: `TRK-XYZ`")
	})

	main.Run("OmitsTrackingLineWhenEmpty", func(t *testing.T) {
		r := newFakeReader()
		rec := sampleRecord()
		rec.Status = "delivered"
		r.put(rec)
		s := &fakeSender{}
		n := newTestNotifier(t, s, r)

		n.Notify(context.Background(), order.NotificationEvent{
			OrderID: rec.ID,
			Status:  "delivered",
		})

		got := waitForCalls(t, s, 1, 2*time.Second)
		assert.NotContains(t, got[0].text, "Tracking:")
	})
}
