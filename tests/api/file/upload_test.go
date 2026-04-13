//go:build functest

package file_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/ashep/simshop/tests/pkg/seeder"
	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadFile(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)
	regularUser := sd.CreateUser(main)

	// Minimal JPEG header that http.DetectContentType recognizes as image/jpeg.
	jpegData := make([]byte, 512)
	jpegData[0] = 0xFF
	jpegData[1] = 0xD8
	jpegData[2] = 0xFF
	jpegData[3] = 0xE0

	doUpload := func(t *testing.T, data []byte, filename string, apiKey string) *http.Response {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, err := mw.CreateFormFile("file", filename)
		require.NoError(t, err)
		_, err = fw.Write(data)
		require.NoError(t, err)
		require.NoError(t, mw.Close())

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, app.URL("/files"), &buf)
		require.NoError(t, err)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		if apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("Success_Admin", func(t *testing.T) {
		t.Parallel()
		resp := doUpload(t, jpegData, "test.jpg", admin.APIKey)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.NotEmpty(t, body["id"])
	})

	main.Run("Success_RegularUser", func(t *testing.T) {
		t.Parallel()
		resp := doUpload(t, jpegData, "test.jpg", regularUser.APIKey)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.NotEmpty(t, body["id"])
	})

	main.Run("Forbidden_Unauthenticated", func(t *testing.T) {
		t.Parallel()
		resp := doUpload(t, jpegData, "test.jpg", "")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("FileTooLarge", func(t *testing.T) {
		t.Parallel()
		// 10 MB + 1 byte
		largeData := make([]byte, 10*1024*1024+1)
		largeData[0] = 0xFF
		largeData[1] = 0xD8
		largeData[2] = 0xFF
		largeData[3] = 0xE0
		resp := doUpload(t, largeData, "large.jpg", admin.APIKey)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "file too large", body["error"])
	})

	main.Run("UnsupportedFileType", func(t *testing.T) {
		t.Parallel()
		textData := []byte("hello, this is plain text")
		resp := doUpload(t, textData, "test.txt", admin.APIKey)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "unsupported file type", body["error"])
	})

	main.Run("MissingFileField", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		require.NoError(t, mw.Close())
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, app.URL("/files"), &buf)
		require.NoError(t, err)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("X-API-Key", admin.APIKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "file field is required", body["error"])
	})

	main.Run("FileLimitReached_RegularUser", func(t *testing.T) {
		t.Parallel()
		u := sd.CreateUser(t)
		for i := 0; i < 50; i++ {
			sd.CreateFile(t, u.ID)
		}
		resp := doUpload(t, jpegData, "test.jpg", u.APIKey)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "file limit reached", body["error"])
	})

	main.Run("FileLimitReached_AdminBypass", func(t *testing.T) {
		t.Parallel()
		adminUser := sd.GetAdminUser(t)
		for i := 0; i < 50; i++ {
			sd.CreateFile(t, adminUser.ID)
		}
		resp := doUpload(t, jpegData, "test.jpg", adminUser.APIKey)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.NotEmpty(t, body["id"])
	})
}
