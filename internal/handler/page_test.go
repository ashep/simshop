package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/internal/page"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type pageServiceMock struct {
	mock.Mock
}

func (m *pageServiceMock) List(ctx context.Context) ([]*page.Page, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*page.Page), args.Error(1)
}

func TestListPages(main *testing.T) {
	resp := buildTestResponder(main)

	doRequest := func(t *testing.T, pageSvc *pageServiceMock) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{pages: pageSvc, resp: resp, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/pages", nil)
		w := httptest.NewRecorder()
		h.ListPages(w, r)
		return w
	}

	main.Run("EmptyList", func(t *testing.T) {
		pageSvc := &pageServiceMock{}
		defer pageSvc.AssertExpectations(t)
		pageSvc.On("List", mock.Anything).Return([]*page.Page{}, nil)

		w := doRequest(t, pageSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.NotNil(t, body)
		assert.Len(t, body, 0)
	})

	main.Run("ReturnsList", func(t *testing.T) {
		pageSvc := &pageServiceMock{}
		defer pageSvc.AssertExpectations(t)
		pageSvc.On("List", mock.Anything).Return([]*page.Page{
			{ID: "about", Title: map[string]string{"en": "About", "uk": "Про нас"}},
		}, nil)

		w := doRequest(t, pageSvc)
		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body, 1)
		assert.Equal(t, "about", body[0]["id"])
		title, ok := body[0]["title"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "About", title["en"])
	})
}

func TestServePage(main *testing.T) {
	dataDir := main.TempDir()
	pageDir := filepath.Join(dataDir, "pages", "about")
	require.NoError(main, os.MkdirAll(pageDir, 0755))
	require.NoError(main, os.WriteFile(filepath.Join(pageDir, "en.md"), []byte("# About"), 0644))

	doRequest := func(t *testing.T, id, lang string) *httptest.ResponseRecorder {
		t.Helper()
		h := &Handler{dataDir: dataDir, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/pages/"+id+"/"+lang, nil)
		r.SetPathValue("id", id)
		r.SetPathValue("lang", lang)
		w := httptest.NewRecorder()
		h.ServePage(w, r)
		return w
	}

	main.Run("ReturnsContent", func(t *testing.T) {
		w := doRequest(t, "about", "en")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, "# About", w.Body.String())
	})

	main.Run("NotFoundWhenIDMissing", func(t *testing.T) {
		w := doRequest(t, "no-such-page", "en")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundWhenLangMissing", func(t *testing.T) {
		w := doRequest(t, "about", "uk")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnIDPathTraversal", func(t *testing.T) {
		h := &Handler{dataDir: dataDir, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/pages/about/en", nil)
		r.SetPathValue("id", "../about") // would resolve to "about" if traversal were allowed
		r.SetPathValue("lang", "en")
		w := httptest.NewRecorder()
		h.ServePage(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	main.Run("NotFoundOnLangPathTraversal", func(t *testing.T) {
		h := &Handler{dataDir: dataDir, l: zerolog.Nop()}
		r := httptest.NewRequest(http.MethodGet, "/pages/about/en", nil)
		r.SetPathValue("id", "about")
		r.SetPathValue("lang", "../en") // would resolve to "en" if traversal were allowed
		w := httptest.NewRecorder()
		h.ServePage(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
