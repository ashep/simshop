package order

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type writerMock struct{ mock.Mock }

func (m *writerMock) Write(ctx context.Context, o Order) (string, error) {
	args := m.Called(ctx, o)
	return args.String(0), args.Error(1)
}

type readerMock struct{ mock.Mock }

func (m *readerMock) List(ctx context.Context) ([]Record, error) {
	args := m.Called(ctx)
	v, _ := args.Get(0).([]Record)
	return v, args.Error(1)
}

func (m *readerMock) GetByID(ctx context.Context, id string) (*Record, error) {
	args := m.Called(ctx, id)
	v, _ := args.Get(0).(*Record)
	return v, args.Error(1)
}

func (m *readerMock) GetStatus(ctx context.Context, id string) (string, error) {
	args := m.Called(ctx, id)
	return args.String(0), args.Error(1)
}

type invoiceWriterMock struct{ mock.Mock }

func (m *invoiceWriterMock) AttachInvoice(ctx context.Context, orderID string, inv Invoice) error {
	return m.Called(ctx, orderID, inv).Error(0)
}

type invoiceEventWriterMock struct{ mock.Mock }

func (m *invoiceEventWriterMock) RecordInvoiceEvent(ctx context.Context, evt InvoiceEvent) (string, error) {
	args := m.Called(ctx, evt)
	return args.String(0), args.Error(1)
}

// notifierMock satisfies order.Notifier and records each Notify call.
type notifierMock struct{ mock.Mock }

func (m *notifierMock) Notify(ctx context.Context, evt NotificationEvent) {
	m.Called(ctx, evt)
}

// Compile-time assertion that notifierMock satisfies the interface.
var _ Notifier = (*notifierMock)(nil)

func TestService(main *testing.T) {
	main.Run("SubmitDelegatesToWriterAndReturnsID", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		o := Order{ProductID: "widget"}
		w.On("Write", mock.Anything, o).Return("018f4e3a-0000-7000-8000-000000000001", nil)

		svc := NewService(w, r, iw, iew, nil, nil)
		id, err := svc.Submit(context.Background(), o)
		require.NoError(t, err)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000001", id)
		w.AssertExpectations(t)
	})

	main.Run("SubmitReturnsWriterError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		o := Order{ProductID: "widget"}
		w.On("Write", mock.Anything, o).Return("", errors.New("write failed"))

		svc := NewService(w, r, iw, iew, nil, nil)
		_, err := svc.Submit(context.Background(), o)
		assert.EqualError(t, err, "write failed")
		w.AssertExpectations(t)
	})

	main.Run("AttachInvoiceDelegatesToInvoiceWriter", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		inv := Invoice{Provider: "monobank", ID: "inv-1", PageURL: "https://pay/inv-1", Amount: 100, Currency: "UAH"}
		iw.On("AttachInvoice", mock.Anything, "order-1", inv).Return(nil)

		svc := NewService(w, r, iw, iew, nil, nil)
		require.NoError(t, svc.AttachInvoice(context.Background(), "order-1", inv))
		iw.AssertExpectations(t)
	})

	main.Run("AttachInvoiceReturnsWriterError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		inv := Invoice{Provider: "monobank"}
		iw.On("AttachInvoice", mock.Anything, "order-1", inv).Return(errors.New("attach failed"))

		svc := NewService(w, r, iw, iew, nil, nil)
		assert.EqualError(t, svc.AttachInvoice(context.Background(), "order-1", inv), "attach failed")
		iw.AssertExpectations(t)
	})

	main.Run("ListDelegatesToReader", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		want := []Record{{ID: "018f4e3a-0000-7000-8000-000000000001", ProductID: "widget"}}
		r.On("List", mock.Anything).Return(want, nil)

		svc := NewService(w, r, iw, iew, nil, nil)
		got, err := svc.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got)
		r.AssertExpectations(t)
	})

	main.Run("ListReturnsReaderError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		r.On("List", mock.Anything).Return(([]Record)(nil), errors.New("read failed"))

		svc := NewService(w, r, iw, iew, nil, nil)
		_, err := svc.List(context.Background())
		assert.EqualError(t, err, "read failed")
		r.AssertExpectations(t)
	})

	main.Run("GetStatusDelegatesToReader", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		r.On("GetStatus", mock.Anything, "order-1").Return("awaiting_payment", nil)

		svc := NewService(w, r, iw, iew, nil, nil)
		got, err := svc.GetStatus(context.Background(), "order-1")
		require.NoError(t, err)
		assert.Equal(t, "awaiting_payment", got)
		r.AssertExpectations(t)
	})

	main.Run("GetStatusReturnsReaderError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		r.On("GetStatus", mock.Anything, "missing").Return("", ErrNotFound)

		svc := NewService(w, r, iw, iew, nil, nil)
		_, err := svc.GetStatus(context.Background(), "missing")
		assert.ErrorIs(t, err, ErrNotFound)
		r.AssertExpectations(t)
	})

	main.Run("RecordInvoiceEventDelegatesToWriter", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		evt := InvoiceEvent{
			OrderID:   "order-1",
			InvoiceID: "inv-1",
			Provider:  "monobank",
			Status:    InvoiceStatusPaid,
			Note:      "monobank: success",
			Payload:   json.RawMessage(`{"status":"success"}`),
		}
		iew.On("RecordInvoiceEvent", mock.Anything, evt).Return("paid", nil)

		svc := NewService(w, r, iw, iew, nil, nil)
		require.NoError(t, svc.RecordInvoiceEvent(context.Background(), evt))
		iew.AssertExpectations(t)
	})

	main.Run("RecordInvoiceEventReturnsWriterError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		evt := InvoiceEvent{OrderID: "order-1", Status: InvoiceStatusPaid}
		iew.On("RecordInvoiceEvent", mock.Anything, evt).Return("", errors.New("record failed"))

		svc := NewService(w, r, iw, iew, nil, nil)
		assert.EqualError(t, svc.RecordInvoiceEvent(context.Background(), evt), "record failed")
		iew.AssertExpectations(t)
	})

	main.Run("SubmitDispatchesNotifyOnSuccess", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		n := &notifierMock{}
		o := Order{ProductID: "widget"}
		w.On("Write", mock.Anything, o).Return("018f4e3a-0000-7000-8000-000000000001", nil)
		n.On("Notify", mock.Anything, NotificationEvent{
			OrderID: "018f4e3a-0000-7000-8000-000000000001",
			Status:  "new",
		}).Return()

		svc := NewService(w, r, iw, iew, nil, n)
		id, err := svc.Submit(context.Background(), o)
		require.NoError(t, err)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000001", id)
		n.AssertExpectations(t)
	})

	main.Run("SubmitDoesNotNotifyOnError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		n := &notifierMock{}
		o := Order{ProductID: "widget"}
		w.On("Write", mock.Anything, o).Return("", errors.New("write failed"))

		svc := NewService(w, r, iw, iew, nil, n)
		_, err := svc.Submit(context.Background(), o)
		assert.Error(t, err)
		n.AssertNotCalled(t, "Notify", mock.Anything, mock.Anything)
	})

	main.Run("SubmitWithNilNotifierDoesNotPanic", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		o := Order{ProductID: "widget"}
		w.On("Write", mock.Anything, o).Return("018f4e3a-0000-7000-8000-000000000099", nil)

		svc := NewService(w, r, iw, iew, nil, nil)
		id, err := svc.Submit(context.Background(), o)
		require.NoError(t, err)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000099", id)
	})

	main.Run("AttachInvoiceDispatchesNotifyOnSuccess", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		n := &notifierMock{}
		inv := Invoice{Provider: "monobank", ID: "inv-1"}
		iw.On("AttachInvoice", mock.Anything, "order-1", inv).Return(nil)
		n.On("Notify", mock.Anything, NotificationEvent{
			OrderID: "order-1",
			Status:  "awaiting_payment",
		}).Return()

		svc := NewService(w, r, iw, iew, nil, n)
		require.NoError(t, svc.AttachInvoice(context.Background(), "order-1", inv))
		n.AssertExpectations(t)
	})

	main.Run("AttachInvoiceDoesNotNotifyOnError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		n := &notifierMock{}
		inv := Invoice{Provider: "monobank"}
		iw.On("AttachInvoice", mock.Anything, "order-1", inv).Return(errors.New("attach failed"))

		svc := NewService(w, r, iw, iew, nil, n)
		assert.Error(t, svc.AttachInvoice(context.Background(), "order-1", inv))
		n.AssertNotCalled(t, "Notify", mock.Anything, mock.Anything)
	})

	main.Run("RecordInvoiceEventDispatchesNotifyOnTransition", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		n := &notifierMock{}
		evt := InvoiceEvent{
			OrderID: "order-1",
			Status:  InvoiceStatusPaid,
			Note:    "monobank: success, finalAmount=100",
			Payload: json.RawMessage(`{"status":"success"}`),
		}
		iew.On("RecordInvoiceEvent", mock.Anything, evt).Return("paid", nil)
		n.On("Notify", mock.Anything, NotificationEvent{
			OrderID: "order-1",
			Status:  "paid",
			Note:    "monobank: success, finalAmount=100",
		}).Return()

		svc := NewService(w, r, iw, iew, nil, n)
		require.NoError(t, svc.RecordInvoiceEvent(context.Background(), evt))
		n.AssertExpectations(t)
	})

	main.Run("RecordInvoiceEventDoesNotNotifyWhenNoTransition", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		n := &notifierMock{}
		evt := InvoiceEvent{
			OrderID: "order-1",
			Status:  InvoiceStatusProcessing,
			Payload: json.RawMessage(`{"status":"processing"}`),
		}
		iew.On("RecordInvoiceEvent", mock.Anything, evt).Return("", nil)

		svc := NewService(w, r, iw, iew, nil, n)
		require.NoError(t, svc.RecordInvoiceEvent(context.Background(), evt))
		n.AssertNotCalled(t, "Notify", mock.Anything, mock.Anything)
	})

	main.Run("RecordInvoiceEventDoesNotNotifyOnError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		iew := &invoiceEventWriterMock{}
		n := &notifierMock{}
		evt := InvoiceEvent{OrderID: "order-1", Payload: json.RawMessage(`{}`)}
		iew.On("RecordInvoiceEvent", mock.Anything, evt).Return("", errors.New("record failed"))

		svc := NewService(w, r, iw, iew, nil, n)
		assert.Error(t, svc.RecordInvoiceEvent(context.Background(), evt))
		n.AssertNotCalled(t, "Notify", mock.Anything, mock.Anything)
	})
}

func TestNotificationEvent(main *testing.T) {
	main.Run("ZeroValue", func(t *testing.T) {
		evt := NotificationEvent{}
		assert.Equal(t, "", evt.OrderID)
		assert.Equal(t, "", evt.Status)
		assert.Equal(t, "", evt.Note)
		assert.Equal(t, "", evt.TrackingNumber)
	})
}
