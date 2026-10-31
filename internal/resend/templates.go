package resend

import (
	"bytes"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"gopkg.in/yaml.v3"

	"github.com/ashep/simshop/internal/order"
)

// markdown is the renderer used by Render. The Table extension is enabled so
// templates can lay out order line items in a GFM-style pipe table; the
// default goldmark instance is CommonMark only and would render the pipes as
// literal text.
var markdown = goldmark.New(goldmark.WithExtensions(extension.Table))

// TemplateData is the variable bag passed to every template execution.
// All fields are pre-formatted strings; callers (Notifier) compute Total,
// resolve OrderURL, and render names before invoking Render.
type TemplateData struct {
	OrderID      string
	OrderShortID string
	CustomerName string
	Items        []TemplateItem
	Total        string
	// Promo fields are zero values when the order has no promo code.
	// Subtotal is the pre-discount amount; Discount is formatted like Total.
	PromoCode       string
	DiscountPercent int
	Discount        string
	Subtotal        string
	StatusNote      string
	TrackingNumber  string
	ShopName        string
	OrderURL        string
	// SiteURL is the scheme and host of OrderURL (no trailing slash), so a
	// layout can reference shop assets such as `{{ .SiteURL }}/assets/logo.png`.
	SiteURL string
}

// LayoutData is what the optional layout.html sees: every TemplateData field,
// plus the rendered subject, the template language and the Markdown body
// already converted to HTML.
type LayoutData struct {
	TemplateData
	Subject string
	Lang    string
	Content htmltemplate.HTML
}

// TemplateItem is one order line as templates see it: Total is the formatted
// line amount before discount (unit price × Qty).
type TemplateItem struct {
	Title string
	Qty   int
	Attrs []order.Attr
	Total string
}

// TemplateStore holds parsed subject and body templates per (status, lang).
// Lookup is map-of-map; Render falls back to "en" when (status, lang) is
// missing but (status, "en") exists. layout, when set, wraps every rendered
// HTML body.
type TemplateStore struct {
	// parsed[status][lang] = parsedTemplate
	parsed map[string]map[string]*parsedTemplate
	layout *htmltemplate.Template
}

type parsedTemplate struct {
	subject *template.Template
	body    *template.Template
}

// All returns the loaded templates indexed by status. Used by startup
// validation in internal/app.
func (s *TemplateStore) All() map[string]map[string]*parsedTemplate {
	return s.parsed
}

// LoadTemplates walks {dir}/{status}/{lang}.md and parses each file, plus the
// optional {dir}/layout.html (an html/template executed with LayoutData).
// A missing dir returns an empty store and no error (matches the loader's
// "missing optional content is not an error" policy). A malformed template
// is fatal — the caller is expected to surface it at startup.
func LoadTemplates(dir string) (*TemplateStore, error) {
	s := &TemplateStore{parsed: map[string]map[string]*parsedTemplate{}}

	statusEntries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read emails dir: %w", err)
	}

	layout, err := os.ReadFile(filepath.Join(dir, "layout.html"))
	switch {
	case err == nil:
		s.layout, err = htmltemplate.New("layout").Parse(string(layout))
		if err != nil {
			return nil, fmt.Errorf("parse emails/layout.html: %w", err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("read emails/layout.html: %w", err)
	}

	for _, st := range statusEntries {
		if !st.IsDir() {
			continue
		}
		statusDir := filepath.Join(dir, st.Name())
		langEntries, err := os.ReadDir(statusDir)
		if err != nil {
			return nil, fmt.Errorf("read emails/%s: %w", st.Name(), err)
		}
		for _, le := range langEntries {
			if le.IsDir() || !strings.HasSuffix(le.Name(), ".md") {
				continue
			}
			lang := strings.TrimSuffix(le.Name(), ".md")
			data, err := os.ReadFile(filepath.Join(statusDir, le.Name()))
			if err != nil {
				return nil, fmt.Errorf("read emails/%s/%s: %w", st.Name(), le.Name(), err)
			}
			pt, err := parseTemplate(data)
			if err != nil {
				return nil, fmt.Errorf("parse emails/%s/%s: %w", st.Name(), le.Name(), err)
			}
			if _, ok := s.parsed[st.Name()]; !ok {
				s.parsed[st.Name()] = map[string]*parsedTemplate{}
			}
			s.parsed[st.Name()][lang] = pt
		}
	}
	return s, nil
}

// Render returns (subject, html, text, error) for the given (status, lang).
// If (status, lang) is missing but (status, "en") exists, the English
// template is used. If neither exists, an error is returned.
func (s *TemplateStore) Render(status, lang string, data TemplateData) (string, string, string, error) {
	byLang, ok := s.parsed[status]
	if !ok {
		return "", "", "", fmt.Errorf("no template for status %q", status)
	}
	pt, ok := byLang[lang]
	if !ok {
		pt, ok = byLang["en"]
		if !ok {
			return "", "", "", fmt.Errorf("no template for status %q (lang %q or en)", status, lang)
		}
	}

	var subjectBuf bytes.Buffer
	if err := pt.subject.Execute(&subjectBuf, data); err != nil {
		return "", "", "", fmt.Errorf("execute subject: %w", err)
	}
	var bodyBuf bytes.Buffer
	if err := pt.body.Execute(&bodyBuf, data); err != nil {
		return "", "", "", fmt.Errorf("execute body: %w", err)
	}

	var htmlBuf bytes.Buffer
	if err := markdown.Convert(bodyBuf.Bytes(), &htmlBuf); err != nil {
		return "", "", "", fmt.Errorf("render markdown: %w", err)
	}

	if s.layout != nil {
		content := htmltemplate.HTML(htmlBuf.String()) //nolint:gosec // goldmark output; raw HTML in templates is omitted
		htmlBuf.Reset()
		err := s.layout.Execute(&htmlBuf, LayoutData{
			TemplateData: data, Subject: subjectBuf.String(), Lang: lang, Content: content,
		})
		if err != nil {
			return "", "", "", fmt.Errorf("execute layout: %w", err)
		}
	}

	return subjectBuf.String(), htmlBuf.String(), bodyBuf.String(), nil
}

type frontmatter struct {
	Subject string `yaml:"subject"`
}

// parseTemplate splits a template file into its YAML frontmatter and Markdown
// body. The frontmatter must be a YAML block delimited by lines containing
// only "---" at the top of the file.
func parseTemplate(data []byte) (*parsedTemplate, error) {
	fm, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	var meta frontmatter
	if err := yaml.Unmarshal(fm, &meta); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	if strings.TrimSpace(meta.Subject) == "" {
		return nil, fmt.Errorf("frontmatter must declare a non-empty subject")
	}

	subjectTpl, err := template.New("subject").Parse(meta.Subject)
	if err != nil {
		return nil, fmt.Errorf("parse subject template: %w", err)
	}
	bodyTpl, err := template.New("body").Parse(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse body template: %w", err)
	}
	return &parsedTemplate{subject: subjectTpl, body: bodyTpl}, nil
}

// splitFrontmatter extracts the YAML between two `---` lines at the start
// of the file. The body starts immediately after the closing `---` line.
func splitFrontmatter(data []byte) ([]byte, []byte, error) {
	const sep = "---\n"
	s := string(data)
	if !strings.HasPrefix(s, sep) {
		// Tolerate `---\r\n` too.
		if !strings.HasPrefix(s, "---\r\n") {
			return nil, nil, fmt.Errorf("missing leading frontmatter delimiter")
		}
		s = "---\n" + strings.TrimPrefix(s, "---\r\n")
	}
	rest := s[len(sep):]
	end := strings.Index(rest, "\n"+sep)
	if end == -1 {
		// Tolerate `---` at EOF without a trailing newline. Require the
		// preceding newline so a single-line frontmatter value ending in
		// `---` (e.g. `foo: ---`) isn't mistaken for the closing delimiter.
		if strings.HasSuffix(rest, "\n---") {
			end = len(rest) - len("\n---")
			return []byte(rest[:end+1]), nil, nil
		}
		return nil, nil, fmt.Errorf("missing closing frontmatter delimiter")
	}
	fm := rest[:end+1]            // include the trailing newline
	body := rest[end+1+len(sep):] // skip the closing `---\n`
	return []byte(fm), []byte(body), nil
}
