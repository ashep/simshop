package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/internal/file"
	"github.com/ashep/simshop/internal/product"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fileServiceMock struct {
	mock.Mock
}

func (m *fileServiceMock) GetForProduct(ctx context.Context, productID string) ([]file.FileInfo, error) {
	args := m.Called(ctx, productID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]file.FileInfo), args.Error(1)
}

func TestGetProductFiles(main *testing.T) {
	resp := buildTestResponder(main)
	const productID = "018f4e3a-0000-7000-8000-000000000099"

	doRequest := func(t *testing.T, prodSvc *productServiceMock, fileSvc *fileServiceMock, id string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{prod: prodSvc, file: fileSvc, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/products/"+id+"/files", nil)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		h.GetProductFiles(w, r)
		return w
	}

	main.Run("ProductNotFound", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Get", mock.Anything, "00000000-0000-0000-0000-000000000000").
			Return(nil, product.ErrProductNotFound)

		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)

		w := doRequest(t, prodSvc, fileSvc, "00000000-0000-0000-0000-000000000000")
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"product not found"}`, w.Body.String())
	})

	main.Run("EmptyFiles", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Get", mock.Anything, productID).
			Return(&product.Product{ID: productID}, nil)

		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)
		fileSvc.On("GetForProduct", mock.Anything, productID).Return([]file.FileInfo{}, nil)

		w := doRequest(t, prodSvc, fileSvc, productID)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.NotNil(t, body)
		assert.Len(t, body, 0)
	})

	main.Run("WithFiles", func(t *testing.T) {
		prodSvc := &productServiceMock{}
		defer prodSvc.AssertExpectations(t)
		prodSvc.On("Get", mock.Anything, productID).
			Return(&product.Product{ID: productID}, nil)

		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)
		fileSvc.On("GetForProduct", mock.Anything, productID).Return([]file.FileInfo{
			{
				Name:      "image.jpg",
				MimeType:  "image/jpeg",
				SizeBytes: 1024,
				Path:      "/files/" + productID + "/image.jpg",
			},
		}, nil)

		w := doRequest(t, prodSvc, fileSvc, productID)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "image.jpg", body[0]["name"])
		assert.Equal(t, "image/jpeg", body[0]["mime_type"])
		assert.Equal(t, float64(1024), body[0]["size_bytes"])
		assert.Equal(t, "/files/"+productID+"/image.jpg", body[0]["path"])
	})
}
