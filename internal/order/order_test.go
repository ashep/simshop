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

func (m *writerMock) Write(ctx context.Context, o Order) error {
	return m.Called(ctx, o).Error(0)
}

type readerMock struct{ mock.Mock }

func (m *readerMock) List(ctx context.Context) ([]Record, error) {
	args := m.Called(ctx)
	v, _ := args.Get(0).([]Record)
	return v, args.Error(1)
}

func TestService(main *testing.T) {
	main.Run("SubmitDelegatesToWriter", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		o := Order{ProductID: "widget"}
		w.On("Write", mock.Anything, o).Return(nil)

		svc := NewService(w, r)
		require.NoError(t, svc.Submit(context.Background(), o))
		w.AssertExpectations(t)
	})

	main.Run("SubmitReturnsWriterError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		o := Order{ProductID: "widget"}
		w.On("Write", mock.Anything, o).Return(errors.New("write failed"))

		svc := NewService(w, r)
		assert.EqualError(t, svc.Submit(context.Background(), o), "write failed")
		w.AssertExpectations(t)
	})

	main.Run("ListDelegatesToReader", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		want := []Record{{ID: "018f4e3a-0000-7000-8000-000000000001", ProductID: "widget"}}
		r.On("List", mock.Anything).Return(want, nil)

		svc := NewService(w, r)
		got, err := svc.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got)
		r.AssertExpectations(t)
	})

	main.Run("ListReturnsReaderError", func(t *testing.T) {
		w := &writerMock{}
		r := &readerMock{}
		r.On("List", mock.Anything).Return(([]Record)(nil), errors.New("read failed"))

		svc := NewService(w, r)
		_, err := svc.List(context.Background())
		assert.EqualError(t, err, "read failed")
		r.AssertExpectations(t)
	})
}
