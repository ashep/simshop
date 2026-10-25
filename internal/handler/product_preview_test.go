package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/internal/shop"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestPlainText(t *testing.T) {
	t.Run("strips markdown and collapses whitespace", func(t *testing.T) {
		got := plainText("A **great**  product\n\nwith [a link](http://x) and `code`", 200)
		require.Equal(t, "A great product with a link and code", got)
	})
	t.Run("truncates on a word boundary with ellipsis", func(t *testing.T) {
		got := plainText("one two three four five", 12)
		require.Equal(t, "one two…", got)
	})
	t.Run("does not split multibyte runes", func(t *testing.T) {
		// 5 Cyrillic runes; max 3 -> no space, hard cut at 3 runes, stays valid UTF-8.
		got := plainText("абвгд", 3)
		require.Equal(t, "абв…", got)
	})
	t.Run("short text is returned unchanged", func(t *testing.T) {
		require.Equal(t, "hello", plainText("hello", 200))
	})
	t.Run("empty input returns empty", func(t *testing.T) {
		require.Equal(t, "", plainText("", 10))
	})
	t.Run("non-positive max returns empty", func(t *testing.T) {
		require.Equal(t, "", plainText("hello", 0))
		require.Equal(t, "", plainText("hello", -1))
	})
	t.Run("link with parentheses in url keeps only text", func(t *testing.T) {
		require.Equal(t, "see docs", plainText("see [docs](http://x.com/a(b))", 200))
	})
}

func TestChooseLang(t *testing.T) {
	names := map[string]string{"en": "Name", "uk": "Назва"}
	require.Equal(t, "uk", chooseLang("uk", names, "en"))         // query wins
	require.Equal(t, "en", chooseLang("", names, "en"))           // default used
	require.Equal(t, "en", chooseLang("de", names, "en"))         // unknown query -> default
	require.Equal(t, "en", chooseLang("de", names, "fr"))         // unknown default -> first alpha
	require.Equal(t, "", chooseLang("x", map[string]string{}, "")) // nothing available
}

func TestFirstImageURL(t *testing.T) {
	imgs := []product.ImageItem{
		{Type: "video", Preview: "v.mp4", Full: "v.mp4"},
		{Preview: "01-p.png", Full: "01.png"},
	}
	require.Equal(t, "https://host/api/images/abc/01.png",
		firstImageURL(imgs, "https://host/api", "abc"))
	require.Equal(t, "", firstImageURL(nil, "https://host/api", "abc"))
	require.Equal(t, "", firstImageURL(
		[]product.ImageItem{{Type: "video", Full: "v.mp4"}}, "https://host/api", "abc"))
}

func TestCoverImageURL(t *testing.T) {
	imgs := []product.ImageItem{
		{Preview: "01-p.png", Full: "01.png"},
		{Preview: "02-p.png", Full: "02.png"},
	}
	require.Equal(t, "https://host/api/images/abc/01.png",
		coverImageURL(&product.Product{Images: imgs}, "https://host/api", "abc"))
	require.Equal(t, "https://host/api/images/abc/02.png",
		coverImageURL(&product.Product{Images: imgs, Cover: "02-p.png"}, "https://host/api", "abc"))
	require.Equal(t, "https://host/api/images/abc/cover.jpg",
		coverImageURL(&product.Product{Images: imgs, Cover: "cover.jpg"}, "https://host/api", "abc"))
}

func TestShopLangHelpers(t *testing.T) {
	s := &shop.Shop{
		Name:        map[string]string{"uk": "Крамниця", "en": "Shop"},
		Description: map[string]string{"en": "desc", "uk": "опис"},
	}
	require.Equal(t, "en", shopDefaultLang(s)) // alphabetically first
	require.Equal(t, "Shop", shopName(s, "en"))
	require.Equal(t, "Крамниця", shopName(s, "uk"))
	require.Equal(t, "Shop", shopName(s, "de")) // fallback to first alpha
	require.Equal(t, "desc", shopDesc(s, "en"))
	require.Equal(t, "", shopDefaultLang(nil))
	require.Equal(t, "", shopName(nil, "en"))
	require.Equal(t, "", shopDesc(nil, "en"))
}

func writeProductYAML(t *testing.T, dir, id, body string) {
	t.Helper()
	pdir := filepath.Join(dir, "products", id)
	require.NoError(t, os.MkdirAll(pdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pdir, "product.yaml"), []byte(body), 0o644))
}

// previewRequest builds a crawler request for the path form the router serves, with the
// id/lang path values set the way the mux would set them.
func previewRequest(id, lang string) *http.Request {
	path := "/product/" + id
	if lang != "" {
		path = "/product/" + lang + "/" + id
	}
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.SetPathValue("id", id)
	r.SetPathValue("lang", lang)
	return r
}

func newPreviewHandler(t *testing.T, dataDir string, s *shop.Shop) *Handler {
	t.Helper()
	return &Handler{
		dataDir:   dataDir,
		publicURL: "https://host/api",
		shop:      &shopServiceStub{shop: s},
		l:         zerolog.Nop(),
	}
}

func TestServeProductPreview(main *testing.T) {
	s := &shop.Shop{
		Name:        map[string]string{"en": "Test Shop", "uk": "Test Shop"},
		Description: map[string]string{"en": "Shop desc", "uk": "Опис"},
	}

	main.Run("renders product OG tags with absolute image and url", func(t *testing.T) {
		dir := t.TempDir()
		writeProductYAML(t, dir, "p1", `
name:
  en: 'Cool "Clock"'
  uk: Годинник
description:
  en: A **great** clock
images:
  - preview: 01-p.png
    full: 01.png
`)
		h := newPreviewHandler(t, dir, s)
		w := httptest.NewRecorder()
		r := previewRequest("p1", "en")

		h.ServeProductPreview(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Header().Get("Content-Type"), "text/html")
		body := w.Body.String()
		require.Contains(t, body, `<meta property="og:title" content="Cool &#34;Clock&#34;">`)
		require.Contains(t, body, `<meta property="og:description" content="A great clock">`)
		require.Contains(t, body, `<meta property="og:image" content="https://host/api/images/p1/01.png">`)
		require.Contains(t, body, `<meta property="og:url" content="https://host/en/product/p1">`)
		require.Contains(t, body, `<meta name="twitter:card" content="summary_large_image">`)
		require.Contains(t, body, `<meta http-equiv="refresh" content="0; url=/en/product/p1">`)
	})

	main.Run("omits og:image when product has no still image", func(t *testing.T) {
		dir := t.TempDir()
		writeProductYAML(t, dir, "p2", `
name:
  en: NoImage
description:
  en: text
images:
  - type: video
    preview: v.mp4
    full: v.mp4
`)
		h := newPreviewHandler(t, dir, s)
		w := httptest.NewRecorder()
		r := previewRequest("p2", "en")

		h.ServeProductPreview(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		require.NotContains(t, w.Body.String(), "og:image")
	})

	main.Run("falls back to shop OG on unknown id, still 200", func(t *testing.T) {
		h := newPreviewHandler(t, t.TempDir(), s)
		w := httptest.NewRecorder()
		r := previewRequest("missing", "")

		h.ServeProductPreview(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		require.Contains(t, body, `<meta property="og:title" content="Test Shop">`)
		require.Contains(t, body, `<meta http-equiv="refresh" content="0; url=/">`)
	})

	main.Run("falls back to shop default lang when path lang absent", func(t *testing.T) {
		dir := t.TempDir()
		writeProductYAML(t, dir, "p3", `
name:
  en: EnglishName
  uk: УкрName
description:
  en: en desc
  uk: укр опис
images: []
`)
		h := newPreviewHandler(t, dir, s)
		w := httptest.NewRecorder()
		r := previewRequest("p3", "") // no lang

		h.ServeProductPreview(w, r)

		body := w.Body.String()
		require.Contains(t, body, `content="EnglishName"`) // shop default "en" wins
		require.Contains(t, body, `<meta property="og:url" content="https://host/en/product/p3">`)
	})

	main.Run("unknown lang behaves like no lang", func(t *testing.T) {
		dir := t.TempDir()
		writeProductYAML(t, dir, "p5", `
name:
  en: EnglishName
  uk: УкрName
description:
  en: en desc
images: []
`)
		h := newPreviewHandler(t, dir, s)
		w := httptest.NewRecorder()

		h.ServeProductPreview(w, previewRequest("p5", "xx"))

		body := w.Body.String()
		require.Contains(t, body, `content="EnglishName"`)
		require.Contains(t, body, `<meta property="og:url" content="https://host/en/product/p5">`)
		require.Contains(t, body, `<meta http-equiv="refresh" content="0; url=/en/product/p5">`)
	})

	main.Run("missing id falls back to shop OG, still 200", func(t *testing.T) {
		h := newPreviewHandler(t, t.TempDir(), s)
		w := httptest.NewRecorder()

		h.ServeProductPreview(w, previewRequest("", ""))

		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), `<meta http-equiv="refresh" content="0; url=/">`)
	})

	main.Run("rejects path-traversal id via shop fallback", func(t *testing.T) {
		h := newPreviewHandler(t, t.TempDir(), s)
		w := httptest.NewRecorder()
		r := previewRequest("../../etc", "")

		h.ServeProductPreview(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), `<meta http-equiv="refresh" content="0; url=/">`)
	})

	main.Run("og:url uses shop origin derived from redirect url", func(t *testing.T) {
		dir := t.TempDir()
		writeProductYAML(t, dir, "p4", `
name:
  en: RedirectHost
description:
  en: text
images: []
`)
		h := &Handler{
			dataDir:     dir,
			publicURL:   "https://api.example.com", // separate API host, no /api suffix
			redirectURL: "https://shop.example.com/order/status",
			shop:        &shopServiceStub{shop: s},
			l:           zerolog.Nop(),
		}
		w := httptest.NewRecorder()
		r := previewRequest("p4", "en")

		h.ServeProductPreview(w, r)

		body := w.Body.String()
		// og:url is on the SHOP host derived from redirect_url...
		require.Contains(t, body, `<meta property="og:url" content="https://shop.example.com/en/product/p4">`)
	})
}
