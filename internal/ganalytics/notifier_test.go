package ganalytics

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/order"
)

type fakeReader struct {
	records map[string]*order.Record
}

func (r *fakeReader) List(context.Context, []string) ([]order.Record, error) {
	return nil, errors.New("not used")
}

func (r *fakeReader) GetByID(_ context.Context, id string) (*order.Record, error) {
	rec, ok := r.records[id]
	if !ok {
		return nil, order.ErrNotFound
	}
	return rec, nil
}

func (r *fakeReader) GetStatus(context.Context, string) (string, error) {
	return "", errors.New("not used")
}

type fakeSender struct {
	mu    sync.Mutex
	calls []Payload
	errs  []error // popped per call; empty = success
}

func (s *fakeSender) Send(_ context.Context, p Payload) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, p)
	if len(s.errs) == 0 {
		return nil
	}
	e := s.errs[0]
	s.errs = s.errs[1:]
	return e
}

type fakeProducts map[string]string

func (p fakeProducts) Title(id string) string { return p[id] }

func sampleRecord() *order.Record {
	return &order.Record{
		ID:          "018f4e3a-0000-7000-8000-000000000001",
		ProductID:   "oak-shelf",
		Price:       240000,
		Currency:    "uah",
		GAClientID:  "1234567890.1761400000",
		GASessionID: "1761400000",
		Attrs: []order.Attr{
			{Name: "Base", Value: "Graphite", Price: 0},
		},
	}
}

// runNotifier feeds events through a started notifier and drains it, so the
// sender's calls are final when it returns.
func runNotifier(t *testing.T, rec *order.Record, s *fakeSender, evts ...order.NotificationEvent) {
	t.Helper()
	n := NewNotifier(s, &fakeReader{records: map[string]*order.Record{rec.ID: rec}},
		fakeProducts{"oak-shelf": "Oak Shelf"}, zerolog.Nop())
	n.sleepFn = func(time.Duration) {}
	n.Start()
	for _, e := range evts {
		n.Notify(t.Context(), e)
	}
	n.Stop()
}

func TestNotifier(main *testing.T) {
	main.Run("PaidSendsPurchase", func(t *testing.T) {
		rec := sampleRecord()
		rec.Promo = &order.Promo{Code: "SPRING15", Percent: 15, Discount: 42000}
		s := &fakeSender{}
		runNotifier(t, rec, s, order.NotificationEvent{OrderID: rec.ID, Status: "paid"})

		require.Len(t, s.calls, 1)
		p := s.calls[0]
		assert.Equal(t, "1234567890.1761400000", p.ClientID)
		require.Len(t, p.Events, 1)
		e := p.Events[0]
		assert.Equal(t, "purchase", e.Name)
		assert.Equal(t, rec.ID, e.Params["transaction_id"])
		assert.Equal(t, 2400.0, e.Params["value"])
		assert.Equal(t, "UAH", e.Params["currency"])
		assert.Equal(t, "SPRING15", e.Params["coupon"])
		assert.Equal(t, "1761400000", e.Params["session_id"])
		assert.Equal(t, []Item{{
			ItemID:      "oak-shelf",
			ItemName:    "Oak Shelf",
			ItemVariant: "Graphite",
			Price:       2400.0,
			Quantity:    1,
		}}, e.Params["items"])
	})

	main.Run("RefundedSendsRefund", func(t *testing.T) {
		rec := sampleRecord()
		s := &fakeSender{}
		runNotifier(t, rec, s, order.NotificationEvent{OrderID: rec.ID, Status: "refunded"})

		require.Len(t, s.calls, 1)
		e := s.calls[0].Events[0]
		assert.Equal(t, "refund", e.Name)
		assert.Equal(t, rec.ID, e.Params["transaction_id"])
		assert.Equal(t, 2400.0, e.Params["value"])
	})

	main.Run("OtherStatusesIgnored", func(t *testing.T) {
		rec := sampleRecord()
		s := &fakeSender{}
		runNotifier(t, rec, s,
			order.NotificationEvent{OrderID: rec.ID, Status: "new"},
			order.NotificationEvent{OrderID: rec.ID, Status: "awaiting_payment"},
			order.NotificationEvent{OrderID: rec.ID, Status: "shipped"},
		)
		assert.Empty(t, s.calls)
	})

	main.Run("NoAnalyticsIDsFallsBackToOrderID", func(t *testing.T) {
		rec := sampleRecord()
		rec.GAClientID, rec.GASessionID = "", ""
		rec.Promo = nil
		s := &fakeSender{}
		runNotifier(t, rec, s, order.NotificationEvent{OrderID: rec.ID, Status: "paid"})

		require.Len(t, s.calls, 1)
		assert.Equal(t, rec.ID, s.calls[0].ClientID)
		params := s.calls[0].Events[0].Params
		assert.NotContains(t, params, "session_id")
		assert.NotContains(t, params, "coupon")
	})

	main.Run("UnknownProductTitleFallsBackToID", func(t *testing.T) {
		rec := sampleRecord()
		rec.ProductID = "gone"
		s := &fakeSender{}
		runNotifier(t, rec, s, order.NotificationEvent{OrderID: rec.ID, Status: "paid"})

		require.Len(t, s.calls, 1)
		assert.Equal(t, "gone", s.calls[0].Events[0].Params["items"].([]Item)[0].ItemName)
	})

	main.Run("TransientErrorRetried", func(t *testing.T) {
		rec := sampleRecord()
		s := &fakeSender{errs: []error{&APIError{HTTPStatus: http.StatusBadGateway}, errors.New("timeout")}}
		runNotifier(t, rec, s, order.NotificationEvent{OrderID: rec.ID, Status: "paid"})
		assert.Len(t, s.calls, 3)
	})

	main.Run("PermanentErrorNotRetried", func(t *testing.T) {
		rec := sampleRecord()
		s := &fakeSender{errs: []error{&APIError{HTTPStatus: http.StatusForbidden}}}
		runNotifier(t, rec, s, order.NotificationEvent{OrderID: rec.ID, Status: "paid"})
		assert.Len(t, s.calls, 1)
	})

	main.Run("MissingOrderDropped", func(t *testing.T) {
		s := &fakeSender{}
		runNotifier(t, sampleRecord(), s, order.NotificationEvent{OrderID: "other", Status: "paid"})
		assert.Empty(t, s.calls)
	})
}
