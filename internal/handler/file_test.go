package handler

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/file"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fileServiceMock struct {
	mock.Mock
}

func (m *fileServiceMock) Upload(ctx context.Context, req file.UploadRequest) (*file.File, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*file.File), args.Error(1)
}

func TestUploadFile(main *testing.T) {
	const maxSize = 1000

	// Minimal JPEG header — enough for http.DetectContentType to return "image/jpeg",
	// small enough to pass the fh.Size > maxSize check in the handler.
	jpegData := make([]byte, 10)
	jpegData[0] = 0xFF
	jpegData[1] = 0xD8
	jpegData[2] = 0xFF
	jpegData[3] = 0xE0

	resp := buildTestResponder(main)

	makeHandler := func(fileSvc *fileServiceMock) *Handler {
		return &Handler{
			file:           fileSvc,
			fileMaxSize:    maxSize,
			fileAllowedMTs: []string{"image/jpeg"},
			resp:           resp,
			l:              zerolog.Nop(),
		}
	}

	buildMultipart := func(t *testing.T, data []byte, filename string, name string) (*bytes.Buffer, string) {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, err := mw.CreateFormFile("file", filename)
		require.NoError(t, err)
		_, err = fw.Write(data)
		require.NoError(t, err)
		if name != "" {
			require.NoError(t, mw.WriteField("name", name))
		}
		require.NoError(t, mw.Close())
		return &buf, mw.FormDataContentType()
	}

	doRequest := func(t *testing.T, h *Handler, body *bytes.Buffer, ct string, user *auth.User) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/files", body)
		r.Header.Set("Content-Type", ct)
		if user != nil {
			r = r.WithContext(auth.ContextWithUser(r.Context(), user))
		}
		w := httptest.NewRecorder()
		h.UploadFile(w, r)
		return w
	}

	admin := &auth.User{ID: "admin-1", Scopes: []auth.Scope{auth.ScopeAdmin}}
	regularUser := &auth.User{ID: "user-1", Scopes: nil}

	main.Run("Unauthenticated", func(t *testing.T) {
		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)

		body, ct := buildMultipart(t, jpegData, "test.jpg", "my-file")
		w := doRequest(t, makeHandler(fileSvc), body, ct, nil)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	main.Run("FileTooLarge_MaxBytesReader", func(t *testing.T) {
		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)

		// 3000-byte body → total multipart > maxSize+1024 = 2024, triggers MaxBytesReader.
		largeData := make([]byte, 3000)
		body, ct := buildMultipart(t, largeData, "large.bin", "")

		w := doRequest(t, makeHandler(fileSvc), body, ct, admin)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"file too large"}`, w.Body.String())
	})

	main.Run("MissingFileField", func(t *testing.T) {
		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)

		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		require.NoError(t, mw.Close())

		w := doRequest(t, makeHandler(fileSvc), &buf, mw.FormDataContentType(), admin)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"file field is required"}`, w.Body.String())
	})

	main.Run("MissingNameField", func(t *testing.T) {
		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)

		body, ct := buildMultipart(t, jpegData, "test.jpg", "")
		w := doRequest(t, makeHandler(fileSvc), body, ct, admin)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"name field is required"}`, w.Body.String())
	})

	main.Run("FileTooLarge_SizeCheck", func(t *testing.T) {
		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)

		// 1100 bytes: total body < maxSize+1024 = 2024 so MaxBytesReader passes,
		// but fh.Size (1100) > maxSize (1000) so the explicit size check fires.
		data := make([]byte, 1100)
		body, ct := buildMultipart(t, data, "medium.bin", "my-file")

		w := doRequest(t, makeHandler(fileSvc), body, ct, admin)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"file too large"}`, w.Body.String())
	})

	main.Run("UnsupportedFileType", func(t *testing.T) {
		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)

		textData := []byte("hello, plain text")
		body, ct := buildMultipart(t, textData, "test.txt", "my-file")

		w := doRequest(t, makeHandler(fileSvc), body, ct, admin)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"unsupported file type"}`, w.Body.String())
	})

	main.Run("FileLimitReached", func(t *testing.T) {
		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)
		fileSvc.On("Upload", mock.Anything, mock.Anything).Return(nil, file.ErrFileLimitReached)

		body, ct := buildMultipart(t, jpegData, "test.jpg", "my-file")
		w := doRequest(t, makeHandler(fileSvc), body, ct, regularUser)

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.JSONEq(t, `{"error":"file limit reached"}`, w.Body.String())
	})

	main.Run("ServiceError", func(t *testing.T) {
		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)
		fileSvc.On("Upload", mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

		body, ct := buildMultipart(t, jpegData, "test.jpg", "my-file")
		w := doRequest(t, makeHandler(fileSvc), body, ct, admin)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	main.Run("Success", func(t *testing.T) {
		fileSvc := &fileServiceMock{}
		defer fileSvc.AssertExpectations(t)
		fileSvc.On("Upload", mock.Anything, mock.MatchedBy(func(req file.UploadRequest) bool {
			return req.Name == "my-file" && req.OwnerID == admin.ID && req.MimeType == "image/jpeg"
		})).Return(&file.File{ID: "018f4e3a-0000-7000-8000-000000000001"}, nil)

		body, ct := buildMultipart(t, jpegData, "test.jpg", "my-file")
		w := doRequest(t, makeHandler(fileSvc), body, ct, admin)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.JSONEq(t, `{"id":"018f4e3a-0000-7000-8000-000000000001"}`, w.Body.String())
	})
}
