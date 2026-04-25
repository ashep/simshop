package order

import (
	"context"
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

type invoiceWriterMock struct{ mock.Mock }

func (m *invoiceWriterMock) AttachInvoice(ctx context.Context, orderID string, inv Invoice) error {
	return m.Called(ctx, orderID, inv).Error(0)
}

func TestService(main *testing.T) {
	main.Run("SubmitDelegatesToWriterAndReturnsID", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		o := Order{ProductID: "widget"}
		w.On("Write", mock.Anything, o).Return("018f4e3a-0000-7000-8000-000000000001", nil)

		svc := NewService(w, r, iw)
		id, err := svc.Submit(context.Background(), o)
		require.NoError(t, err)
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000001", id)
		w.AssertExpectations(t)
	})

	main.Run("SubmitReturnsWriterError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		o := Order{ProductID: "widget"}
		w.On("Write", mock.Anything, o).Return("", errors.New("write failed"))

		svc := NewService(w, r, iw)
		_, err := svc.Submit(context.Background(), o)
		assert.EqualError(t, err, "write failed")
		w.AssertExpectations(t)
	})

	main.Run("AttachInvoiceDelegatesToInvoiceWriter", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		inv := Invoice{Provider: "monobank", InvoiceID: "inv-1", PageURL: "https://pay/inv-1", Amount: 100, Currency: "UAH"}
		iw.On("AttachInvoice", mock.Anything, "order-1", inv).Return(nil)

		svc := NewService(w, r, iw)
		require.NoError(t, svc.AttachInvoice(context.Background(), "order-1", inv))
		iw.AssertExpectations(t)
	})

	main.Run("AttachInvoiceReturnsWriterError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		inv := Invoice{Provider: "monobank"}
		iw.On("AttachInvoice", mock.Anything, "order-1", inv).Return(errors.New("attach failed"))

		svc := NewService(w, r, iw)
		assert.EqualError(t, svc.AttachInvoice(context.Background(), "order-1", inv), "attach failed")
		iw.AssertExpectations(t)
	})

	main.Run("ListDelegatesToReader", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		want := []Record{{ID: "018f4e3a-0000-7000-8000-000000000001", ProductID: "widget"}}
		r.On("List", mock.Anything).Return(want, nil)

		svc := NewService(w, r, iw)
		got, err := svc.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got)
		r.AssertExpectations(t)
	})

	main.Run("ListReturnsReaderError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		iw := &invoiceWriterMock{}
		r.On("List", mock.Anything).Return(([]Record)(nil), errors.New("read failed"))

		svc := NewService(w, r, iw)
		_, err := svc.List(context.Background())
		assert.EqualError(t, err, "read failed")
		r.AssertExpectations(t)
	})
}
