package metacapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	calls [][]Event
	errs  []error // popped per call; empty = success
}

func (s *fakeSender) Send(_ context.Context, events []Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, events)
	if len(s.errs) == 0 {
		return nil
	}
	e := s.errs[0]
	s.errs = s.errs[1:]
	return e
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func sampleRecord() *order.Record {
	return &order.Record{
		ID:        "018f4e3a-0000-7000-8000-000000000001",
		ProductID: "oak-shelf",
		Price:     240000,
		Currency:  "uah",
		Email:     "  Ivan@Example.COM ",
		Phone:     "+38 (050) 123-45-67",
		FirstName: "Іван",
		LastName:  "Іваненко",
		City:      "Київ",
		Country:   "UA",
		MetaFBP:   "fb.1.1761400000000.123456789",
		MetaFBC:   "fb.1.1761400000000.IwAR0abc",
		ClientIP:  "203.0.113.7",
		UserAgent: "Mozilla/5.0 (iPhone)",
	}
}

var fixedNow = time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)

// runNotifier feeds events through a started notifier and drains it, so the
// sender's calls are final when it returns.
func runNotifier(t *testing.T, rec *order.Record, s *fakeSender, evts ...order.NotificationEvent) {
	t.Helper()
	n := NewNotifier(s, &fakeReader{records: map[string]*order.Record{rec.ID: rec}}, zerolog.Nop())
	n.sleepFn = func(time.Duration) {}
	n.nowFn = func() time.Time { return fixedNow }
	n.Start()
	for _, e := range evts {
		n.Notify(t.Context(), e)
	}
	n.Stop()
}

func TestNotifier(main *testing.T) {
	main.Run("PaidSendsPurchase", func(t *testing.T) {
		rec := sampleRecord()
		s := &fakeSender{}
		runNotifier(t, rec, s, order.NotificationEvent{OrderID: rec.ID, Status: "paid"})

		require.Len(t, s.calls, 1)
		require.Len(t, s.calls[0], 1)
		e := s.calls[0][0]
		assert.Equal(t, "Purchase", e.EventName)
		assert.Equal(t, fixedNow.Unix(), e.EventTime)
		assert.Equal(t, rec.ID, e.EventID)
		assert.Equal(t, "website", e.ActionSource)
		assert.Equal(t, CustomData{
			Currency:    "UAH",
			Value:       2400,
			ContentIDs:  []string{"oak-shelf"},
			ContentType: "product",
			OrderID:     rec.ID,
			NumItems:    1,
		}, e.CustomData)
		assert.Equal(t, UserData{
			Email:           []string{sha("ivan@example.com")},
			Phone:           []string{sha("380501234567")},
			FirstName:       []string{sha("іван")},
			LastName:        []string{sha("іваненко")},
			City:            []string{sha("київ")},
			Country:         []string{sha("ua")},
			ExternalID:      []string{sha("380501234567")},
			FBP:             "fb.1.1761400000000.123456789",
			FBC:             "fb.1.1761400000000.IwAR0abc",
			ClientIPAddress: "203.0.113.7",
			ClientUserAgent: "Mozilla/5.0 (iPhone)",
		}, e.UserData)
	})

	main.Run("CityNormalizedToLettersOnly", func(t *testing.T) {
		rec := sampleRecord()
		rec.City = " Кам'янець-Подільський "
		s := &fakeSender{}
		runNotifier(t, rec, s, order.NotificationEvent{OrderID: rec.ID, Status: "paid"})

		require.Len(t, s.calls, 1)
		assert.Equal(t, []string{sha("камянецьподільський")}, s.calls[0][0].UserData.City)
	})

	main.Run("EmptyFieldsOmitted", func(t *testing.T) {
		rec := sampleRecord()
		rec.Email, rec.MetaFBP, rec.MetaFBC, rec.ClientIP, rec.UserAgent = "", "", "", "", ""
		s := &fakeSender{}
		runNotifier(t, rec, s, order.NotificationEvent{OrderID: rec.ID, Status: "paid"})

		require.Len(t, s.calls, 1)
		ud := s.calls[0][0].UserData
		assert.Nil(t, ud.Email)
		assert.Empty(t, ud.FBP)
		assert.Empty(t, ud.FBC)
		assert.Empty(t, ud.ClientIPAddress)
		assert.Empty(t, ud.ClientUserAgent)
	})

	main.Run("OtherStatusesIgnored", func(t *testing.T) {
		rec := sampleRecord()
		s := &fakeSender{}
		runNotifier(t, rec, s,
			order.NotificationEvent{OrderID: rec.ID, Status: "new"},
			order.NotificationEvent{OrderID: rec.ID, Status: "shipped"},
			order.NotificationEvent{OrderID: rec.ID, Status: "refunded"},
		)
		assert.Empty(t, s.calls)
	})

	main.Run("TransientErrorRetried", func(t *testing.T) {
		rec := sampleRecord()
		s := &fakeSender{errs: []error{&APIError{HTTPStatus: http.StatusInternalServerError}, errors.New("timeout")}}
		runNotifier(t, rec, s, order.NotificationEvent{OrderID: rec.ID, Status: "paid"})
		assert.Len(t, s.calls, 3)
	})

	main.Run("PermanentErrorNotRetried", func(t *testing.T) {
		rec := sampleRecord()
		s := &fakeSender{errs: []error{&APIError{HTTPStatus: http.StatusBadRequest}}}
		runNotifier(t, rec, s, order.NotificationEvent{OrderID: rec.ID, Status: "paid"})
		assert.Len(t, s.calls, 1)
	})

	main.Run("MissingOrderDropped", func(t *testing.T) {
		s := &fakeSender{}
		runNotifier(t, sampleRecord(), s, order.NotificationEvent{OrderID: "other", Status: "paid"})
		assert.Empty(t, s.calls)
	})
}
