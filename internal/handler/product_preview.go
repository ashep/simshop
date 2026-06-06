package handler

import (
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/internal/shop"
	"gopkg.in/yaml.v3"
)

var (
	mdImageOrLink = regexp.MustCompile(`!?\[([^\]]*)\]\((?:[^()]|\([^()]*\))*\)`) // [txt](url) / ![alt](url) -> txt/alt, tolerates one level of parens in the URL
	mdInlineMarks = regexp.MustCompile("[`*_#>~]+")               // emphasis, headings, quotes, code, strike
	whitespaceRun = regexp.MustCompile(`\s+`)
)

// plainText converts markdown to a flat meta-description string, truncated to max runes
// on a word boundary with a trailing ellipsis.
func plainText(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = mdImageOrLink.ReplaceAllString(s, "$1")
	s = mdInlineMarks.ReplaceAllString(s, "")
	s = whitespaceRun.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "…"
}

// firstAlphaLang returns the alphabetically-first key of m, or "" if empty.
func firstAlphaLang(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	return keys[0]
}

// chooseLang picks the preview language: requested if the product has it, else the shop
// default if the product has it, else the product's alphabetically-first language.
func chooseLang(want string, names map[string]string, def string) string {
	if want != "" && names[want] != "" {
		return want
	}
	if def != "" && names[def] != "" {
		return def
	}
	return firstAlphaLang(names)
}

// firstImageURL returns the absolute URL of the first non-video image's Full path, or "".
func firstImageURL(imgs []product.ImageItem, publicURL, id string) string {
	for _, img := range imgs {
		if img.Type == "video" || img.Full == "" {
			continue
		}
		return publicURL + "/images/" + id + "/" + img.Full
	}
	return ""
}

func shopDefaultLang(s *shop.Shop) string {
	if s == nil {
		return ""
	}
	return firstAlphaLang(s.Name)
}

func shopName(s *shop.Shop, lang string) string {
	if s == nil {
		return ""
	}
	if v := s.Name[lang]; v != "" {
		return v
	}
	return s.Name[firstAlphaLang(s.Name)]
}

func shopDesc(s *shop.Shop, lang string) string {
	if s == nil {
		return ""
	}
	if v := s.Description[lang]; v != "" {
		return v
	}
	return s.Description[firstAlphaLang(s.Description)]
}

type ogData struct {
	Title       string
	Description string
	SiteName    string
	Image       string // absolute URL; empty -> og:image/twitter:image omitted
	URL         string
	Locale      string
	Refresh     string // relative URL for the meta-refresh and body link
}

var previewTmpl = template.Must(template.New("preview").Parse(`<!DOCTYPE html>
<html lang="{{.Locale}}">
<head>
<meta charset="utf-8">
<title>{{.Title}}</title>
<meta name="description" content="{{.Description}}">
<meta property="og:type" content="website">
<meta property="og:site_name" content="{{.SiteName}}">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Description}}">
{{if .Image}}<meta property="og:image" content="{{.Image}}">
{{end}}<meta property="og:url" content="{{.URL}}">
<meta property="og:locale" content="{{.Locale}}">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="{{.Title}}">
<meta name="twitter:description" content="{{.Description}}">
{{if .Image}}<meta name="twitter:image" content="{{.Image}}">
{{end}}<link rel="canonical" href="{{.URL}}">
<meta http-equiv="refresh" content="0; url={{.Refresh}}">
</head>
<body><a href="{{.Refresh}}">{{.Title}}</a></body>
</html>`))

// shopOrigin returns the public scheme://host of the human-facing shop. It is derived from
// the Monobank redirect URL (which points at the shop, e.g. https://shop.tld/order/status),
// falling back to the API public URL minus its /api suffix when redirectURL is unset/unparseable.
func (h *Handler) shopOrigin() string {
	if u, err := url.Parse(h.redirectURL); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return strings.TrimSuffix(h.publicURL, "/api")
}

// ServeProductPreview returns a minimal HTML document with Open Graph / Twitter meta tags for
// social-media crawlers. Caddy routes crawler User-Agents on /product here; humans get the static SPA.
// It never returns a non-200 to a crawler: invalid/unknown products fall back to shop-level tags.
func (h *Handler) ServeProductPreview(w http.ResponseWriter, r *http.Request) {
	origin := h.shopOrigin()

	id := r.URL.Query().Get("id")
	if id == "" || id != filepath.Base(id) || id == "." || strings.ContainsAny(id, "\r\n\x00") {
		h.writePreview(w, h.shopPreview(r, origin))
		return
	}

	data, err := os.ReadFile(filepath.Join(h.dataDir, "products", id, "product.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		h.writePreview(w, h.shopPreview(r, origin))
		return
	}
	if err != nil {
		h.l.Warn().Err(err).Str("id", id).Msg("product preview: read product yaml")
		h.writePreview(w, h.shopPreview(r, origin))
		return
	}

	var p product.Product
	if err := yaml.Unmarshal(data, &p); err != nil {
		h.writePreview(w, h.shopPreview(r, origin))
		return
	}

	s, _ := h.shop.Get(r.Context())
	lang := chooseLang(r.URL.Query().Get("lang"), p.Name, shopDefaultLang(s))
	if lang == "" {
		h.writePreview(w, h.shopPreview(r, origin))
		return
	}

	pageURL := origin + "/product?id=" + id + "&lang=" + lang
	h.writePreview(w, ogData{
		Title:       p.Name[lang],
		Description: plainText(p.Description[lang], 200),
		SiteName:    shopName(s, lang),
		Image:       firstImageURL(p.Images, h.publicURL, id),
		URL:         pageURL,
		Locale:      lang,
		Refresh:     "/product?id=" + id + "&lang=" + lang,
	})
}

// shopPreview builds the shop-level fallback tags used when a product can't be resolved.
func (h *Handler) shopPreview(r *http.Request, origin string) ogData {
	s, _ := h.shop.Get(r.Context())
	lang := shopDefaultLang(s)
	return ogData{
		Title:       shopName(s, lang),
		Description: plainText(shopDesc(s, lang), 200),
		SiteName:    shopName(s, lang),
		URL:         origin + "/",
		Locale:      lang,
		Refresh:     "/",
	}
}

func (h *Handler) writePreview(w http.ResponseWriter, og ogData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if err := previewTmpl.Execute(w, og); err != nil {
		h.l.Warn().Err(err).Msg("preview template execute failed")
	}
}
