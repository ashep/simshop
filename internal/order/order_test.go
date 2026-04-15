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

func TestService(main *testing.T) {
	main.Run("SubmitDelegatesToWriter", func(t *testing.T) {
		w := &writerMock{}
		o := Order{ProductName: "Widget"}
		w.On("Write", mock.Anything, o).Return(nil)

		svc := NewService(w)
		require.NoError(t, svc.Submit(context.Background(), o))
		w.AssertExpectations(t)
	})

	main.Run("SubmitReturnsWriterError", func(t *testing.T) {
		w := &writerMock{}
		o := Order{ProductName: "Widget"}
		w.On("Write", mock.Anything, o).Return(errors.New("write failed"))

		svc := NewService(w)
		assert.EqualError(t, svc.Submit(context.Background(), o), "write failed")
		w.AssertExpectations(t)
	})
}
