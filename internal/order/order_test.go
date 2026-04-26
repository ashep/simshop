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

func (m *readerMock) GetStatus(ctx context.Context, id string) (string, error) {
	args := m.Called(ctx, id)
	return args.String(0), args.Error(1)
}

type invoiceWriterMock struct{ mock.Mock }

func (m *invoiceWriterMock) AttachInvoice(ctx context.Context, orderID string, inv Invoice) error {
	return m.Called(ctx, orderID, inv).Error(0)
}

type paymentEventWriterMock struct{ mock.Mock }

func (m *paymentEventWriterMock) ApplyPaymentEvent(ctx context.Context, orderID, status, note string, payload json.RawMessage) error {
	return m.Called(ctx, orderID, status, note, payload).Error(0)
}

func TestService(main *testing.T) {
	main.Run("SubmitDelegatesToWriterAndReturnsID", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		pew := &paymentEventWriterMock{}
		o := Order{ProductID: "widget"}
		w.On("Write", mock.Anything, o).Return("018f4e3a-0000-7000-8000-000000000001", nil)

		svc := NewService(w, r, iw, pew)
		id, err := svc.Submit(context.Background(), o)
		require.NoError(t, err)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000001", id)
		w.AssertExpectations(t)
	})

	main.Run("SubmitReturnsWriterError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		pew := &paymentEventWriterMock{}
		o := Order{ProductID: "widget"}
		w.On("Write", mock.Anything, o).Return("", errors.New("write failed"))

		svc := NewService(w, r, iw, pew)
		_, err := svc.Submit(context.Background(), o)
		assert.EqualError(t, err, "write failed")
		w.AssertExpectations(t)
	})

	main.Run("AttachInvoiceDelegatesToInvoiceWriter", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		pew := &paymentEventWriterMock{}
		inv := Invoice{Provider: "monobank", ID: "inv-1", PageURL: "https://pay/inv-1", Amount: 100, Currency: "UAH"}
		iw.On("AttachInvoice", mock.Anything, "order-1", inv).Return(nil)

		svc := NewService(w, r, iw, pew)
		require.NoError(t, svc.AttachInvoice(context.Background(), "order-1", inv))
		iw.AssertExpectations(t)
	})

	main.Run("AttachInvoiceReturnsWriterError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		pew := &paymentEventWriterMock{}
		inv := Invoice{Provider: "monobank"}
		iw.On("AttachInvoice", mock.Anything, "order-1", inv).Return(errors.New("attach failed"))

		svc := NewService(w, r, iw, pew)
		assert.EqualError(t, svc.AttachInvoice(context.Background(), "order-1", inv), "attach failed")
		iw.AssertExpectations(t)
	})

	main.Run("ListDelegatesToReader", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		pew := &paymentEventWriterMock{}
		want := []Record{{ID: "018f4e3a-0000-7000-8000-000000000001", ProductID: "widget"}}
		r.On("List", mock.Anything).Return(want, nil)

		svc := NewService(w, r, iw, pew)
		got, err := svc.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got)
		r.AssertExpectations(t)
	})

	main.Run("ListReturnsReaderError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		pew := &paymentEventWriterMock{}
		r.On("List", mock.Anything).Return(([]Record)(nil), errors.New("read failed"))

		svc := NewService(w, r, iw, pew)
		_, err := svc.List(context.Background())
		assert.EqualError(t, err, "read failed")
		r.AssertExpectations(t)
	})

	main.Run("GetStatusDelegatesToReader", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		pew := &paymentEventWriterMock{}
		r.On("GetStatus", mock.Anything, "order-1").Return("awaiting_payment", nil)

		svc := NewService(w, r, iw, pew)
		got, err := svc.GetStatus(context.Background(), "order-1")
		require.NoError(t, err)
		assert.Equal(t, "awaiting_payment", got)
		r.AssertExpectations(t)
	})

	main.Run("GetStatusReturnsReaderError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		pew := &paymentEventWriterMock{}
		r.On("GetStatus", mock.Anything, "missing").Return("", ErrNotFound)

		svc := NewService(w, r, iw, pew)
		_, err := svc.GetStatus(context.Background(), "missing")
		assert.ErrorIs(t, err, ErrNotFound)
		r.AssertExpectations(t)
	})

	main.Run("ApplyPaymentEventDelegatesToWriter", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		pew := &paymentEventWriterMock{}
		payload := json.RawMessage(`{"status":"success"}`)
		pew.On("ApplyPaymentEvent", mock.Anything, "order-1", "paid", "monobank: success", payload).Return(nil)

		svc := NewService(w, r, iw, pew)
		require.NoError(t, svc.ApplyPaymentEvent(context.Background(), "order-1", "paid", "monobank: success", payload))
		pew.AssertExpectations(t)
	})

	main.Run("ApplyPaymentEventReturnsWriterError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		pew := &paymentEventWriterMock{}
		pew.On("ApplyPaymentEvent", mock.Anything, "order-1", "paid", "", json.RawMessage(nil)).Return(errors.New("apply failed"))

		svc := NewService(w, r, iw, pew)
		assert.EqualError(t, svc.ApplyPaymentEvent(context.Background(), "order-1", "paid", "", nil), "apply failed")
		pew.AssertExpectations(t)
	})
}
