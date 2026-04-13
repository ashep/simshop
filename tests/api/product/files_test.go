//go:build functest

package product_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/tests/pkg/seeder"
	"github.com/ashep/simshop/tests/pkg/testapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetProductFiles(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	shopOwner := sd.CreateUser(main)
	sh := sd.CreateShop(main, "setfilesshop", shopOwner.ID, map[string]string{"EN": "Set Files Shop"}, nil)

	doRequest := func(t *testing.T, id string, body string, apiKey string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPut,
			app.URL("/products/"+id+"/files"),
			bytes.NewBufferString(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("Success_Admin", func(t *testing.T) {
		t.Parallel()

		otherUser := sd.CreateUser(t)
		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})
		// Admin can attach a file owned by any user
		fileID := sd.CreateFile(t, otherUser.ID)

		body, _ := json.Marshal(map[string]any{"file_ids": []string{fileID}})
		resp := doRequest(t, p.ID, string(body), admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, []string{fileID}, sd.GetProductFiles(t, p.ID))
	})

	main.Run("Success_ShopOwner", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})
		fileID := sd.CreateFile(t, shopOwner.ID)

		body, _ := json.Marshal(map[string]any{"file_ids": []string{fileID}})
		resp := doRequest(t, p.ID, string(body), shopOwner.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, []string{fileID}, sd.GetProductFiles(t, p.ID))
	})

	main.Run("Success_ReplacesExisting", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})
		oldFileID := sd.CreateFile(t, shopOwner.ID)
		newFileID := sd.CreateFile(t, shopOwner.ID)
		sd.SetProductFiles(t, p.ID, []string{oldFileID})

		body, _ := json.Marshal(map[string]any{"file_ids": []string{newFileID}})
		resp := doRequest(t, p.ID, string(body), shopOwner.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, []string{newFileID}, sd.GetProductFiles(t, p.ID))
	})

	main.Run("Success_ClearsAll", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})
		fileID := sd.CreateFile(t, shopOwner.ID)
		sd.SetProductFiles(t, p.ID, []string{fileID})

		body := `{"file_ids":[]}`
		resp := doRequest(t, p.ID, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, sd.GetProductFiles(t, p.ID))
	})

	main.Run("Forbidden_Unauthenticated", func(t *testing.T) {
		t.Parallel()

		body := `{"file_ids":[]}`
		resp := doRequest(t, "00000000-0000-7000-8000-000000000000", body, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("Forbidden_NonOwner", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})
		otherUser := sd.CreateUser(t)

		body := `{"file_ids":[]}`
		resp := doRequest(t, p.ID, body, otherUser.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	main.Run("ProductNotFound", func(t *testing.T) {
		t.Parallel()

		body := `{"file_ids":[]}`
		resp := doRequest(t, "00000000-0000-7000-8000-000000000000", body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"product not found"}`, string(respBody))
	})

	main.Run("FileNotFound", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})

		body := `{"file_ids":["00000000-0000-7000-8000-000000000001"]}`
		resp := doRequest(t, p.ID, body, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"file not found"}`, string(respBody))
	})

	main.Run("FileOwnerMismatch", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "A widget"},
		})
		otherUser := sd.CreateUser(t)
		fileID := sd.CreateFile(t, otherUser.ID)

		// Shop owner tries to attach a file owned by a different user
		body, _ := json.Marshal(map[string]any{"file_ids": []string{fileID}})
		resp := doRequest(t, p.ID, string(body), shopOwner.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

func TestGetProductFiles(main *testing.T) {
	app := testapp.New(main)
	app.Start()

	sd := seeder.New(main, app.DB())
	admin := sd.GetAdminUser(main)

	shopOwner := sd.CreateUser(main)
	sh := sd.CreateShop(main, "getfilesshop", shopOwner.ID, map[string]string{"EN": "Get Files Shop"}, nil)

	doRequest := func(t *testing.T, id string, apiKey string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			app.URL("/products/"+id+"/files"), nil)
		require.NoError(t, err)
		if apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	main.Run("ProductNotFound", func(t *testing.T) {
		t.Parallel()

		resp := doRequest(t, "00000000-0000-7000-8000-000000000000", "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"error":"product not found"}`, string(body))
	})

	main.Run("EmptyList", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "No Files", Description: "Product with no files"},
		})

		resp := doRequest(t, p.ID, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `[]`, string(body))
	})

	main.Run("PublicFields_Anonymous", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "Desc"},
		})
		fileID := sd.CreateFile(t, shopOwner.ID)
		sd.SetProductFiles(t, p.ID, []string{fileID})

		resp := doRequest(t, p.ID, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var items []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&items))
		assert.Len(t, items, 1)
		assert.Contains(t, items[0], "id")
		assert.NotNil(t, items[0]["id"])
		assert.Contains(t, items[0], "name")
		assert.Contains(t, items[0], "mime_type")
		assert.Contains(t, items[0], "size_bytes")
		assert.Contains(t, items[0], "path")
		assert.NotContains(t, items[0], "created_at")
		assert.NotContains(t, items[0], "updated_at")
	})

	main.Run("AdminFields", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "Desc"},
		})
		fileID := sd.CreateFile(t, shopOwner.ID)
		sd.SetProductFiles(t, p.ID, []string{fileID})

		resp := doRequest(t, p.ID, admin.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var items []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&items))
		assert.Len(t, items, 1)
		assert.Contains(t, items[0], "created_at")
		assert.NotNil(t, items[0]["created_at"])
		assert.Contains(t, items[0], "updated_at")
		assert.NotNil(t, items[0]["updated_at"])
	})

	main.Run("OwnerFields", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "Desc"},
		})
		fileID := sd.CreateFile(t, shopOwner.ID)
		sd.SetProductFiles(t, p.ID, []string{fileID})

		resp := doRequest(t, p.ID, shopOwner.APIKey)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var items []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&items))
		assert.Len(t, items, 1)
		assert.Contains(t, items[0], "created_at")
		assert.NotNil(t, items[0]["created_at"])
		assert.Contains(t, items[0], "updated_at")
		assert.NotNil(t, items[0]["updated_at"])
	})

	main.Run("FileMaterialized", func(t *testing.T) {
		t.Parallel()

		p := sd.CreateProduct(t, sh.ID, nil, map[string]product.DataItem{
			"EN": {Title: "Widget", Description: "Desc"},
		})
		fileID := sd.CreateFile(t, shopOwner.ID)
		sd.SetProductFiles(t, p.ID, []string{fileID})

		resp := doRequest(t, p.ID, "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var items []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&items))
		require.Len(t, items, 1)

		// seeder.CreateFile uses name "seeded-file"
		expectedDiskPath := filepath.Join(app.PublicDir(), "files", fileID, "seeded-file")
		_, err := os.Stat(expectedDiskPath)
		assert.NoError(t, err, "file should exist on disk at %s", expectedDiskPath)

		expectedURLPath := "/files/" + fileID + "/seeded-file"
		assert.Equal(t, expectedURLPath, items[0]["path"])
	})
}
