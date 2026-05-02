package resend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTpl(t *testing.T, dir, status, lang, content string) {
	t.Helper()
	sd := filepath.Join(dir, status)
	require.NoError(t, os.MkdirAll(sd, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sd, lang+".md"), []byte(content), 0644))
}

func TestLoadTemplates(main *testing.T) {
	main.Run("EmptyDirReturnsEmptyMap", func(t *testing.T) {
		dir := t.TempDir()
		store, err := LoadTemplates(dir)
		require.NoError(t, err)
		assert.NotNil(t, store)
		assert.Empty(t, store.All())
	})

	main.Run("MissingDirReturnsEmptyMap", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "no-such")
		store, err := LoadTemplates(dir)
		require.NoError(t, err)
		assert.Empty(t, store.All())
	})

	main.Run("LoadsValidTemplate", func(t *testing.T) {
		dir := t.TempDir()
		writeTpl(t, dir, "paid", "en", `---
subject: Order {{ .OrderShortID }} paid
---
Thanks {{ .CustomerName }}.`)
		store, err := LoadTemplates(dir)
		require.NoError(t, err)

		subject, html, text, err := store.Render("paid", "en", TemplateData{
			OrderShortID: "abc-1234567", CustomerName: "Jane",
		})
		require.NoError(t, err)
		assert.Equal(t, "Order abc-1234567 paid", subject)
		assert.Contains(t, html, "Thanks Jane.")
		assert.Equal(t, "Thanks Jane.", strings.TrimSpace(text))
	})

	main.Run("MissingSubjectIsFatal", func(t *testing.T) {
		dir := t.TempDir()
		writeTpl(t, dir, "paid", "en", `---
foo: bar
---
body`)
		_, err := LoadTemplates(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subject")
	})

	main.Run("MissingFrontmatterIsFatal", func(t *testing.T) {
		dir := t.TempDir()
		writeTpl(t, dir, "paid", "en", "no frontmatter here")
		_, err := LoadTemplates(dir)
		require.Error(t, err)
	})

	main.Run("FallsBackToEnglishWhenLangMissing", func(t *testing.T) {
		dir := t.TempDir()
		writeTpl(t, dir, "shipped", "en", `---
subject: Shipped {{ .OrderShortID }}
---
en body`)
		store, err := LoadTemplates(dir)
		require.NoError(t, err)
		subject, html, _, err := store.Render("shipped", "uk", TemplateData{OrderShortID: "x"})
		require.NoError(t, err)
		assert.Equal(t, "Shipped x", subject)
		assert.Contains(t, html, "en body")
	})

	main.Run("ReturnsErrorWhenStatusMissing", func(t *testing.T) {
		store, err := LoadTemplates(t.TempDir())
		require.NoError(t, err)
		_, _, _, err = store.Render("paid", "en", TemplateData{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no template")
	})

	main.Run("PreservesMarkdownFormatting", func(t *testing.T) {
		dir := t.TempDir()
		writeTpl(t, dir, "paid", "en", `---
subject: s
---
**bold** and _italic_`)
		store, err := LoadTemplates(dir)
		require.NoError(t, err)
		_, html, _, err := store.Render("paid", "en", TemplateData{})
		require.NoError(t, err)
		assert.Contains(t, html, "<strong>bold</strong>")
		assert.Contains(t, html, "<em>italic</em>")
	})

	main.Run("ShippedRendersTrackingLineWhenSet", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "shipped"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "shipped", "en.md"),
			[]byte("---\nsubject: x\n---\n{{- if .TrackingNumber }}Tracking: {{ .TrackingNumber }}{{- end }}"),
			0644))

		store, err := LoadTemplates(dir)
		require.NoError(t, err)

		_, html, _, err := store.Render("shipped", "en", TemplateData{TrackingNumber: "TRK-XYZ"})
		require.NoError(t, err)
		assert.Contains(t, html, "TRK-XYZ")

		_, html2, _, err := store.Render("shipped", "en", TemplateData{})
		require.NoError(t, err)
		assert.NotContains(t, html2, "TRK-")
	})
}

func TestTemplateData_AllFieldsExist(t *testing.T) {
	// Compile-time guard against accidental field rename. Every field listed
	// in the docs must exist in the struct; if any is renamed without
	// updating this literal, the build breaks.
	d := TemplateData{
		OrderID: "x", OrderShortID: "x", CustomerName: "x", ProductTitle: "x",
		Total: "x", StatusNote: "x", TrackingNumber: "x", ShopName: "x", OrderURL: "x",
		Attrs: nil,
	}
	assert.Equal(t, "x", d.OrderID)
	assert.Equal(t, "x", d.OrderShortID)
	assert.Equal(t, "x", d.CustomerName)
	assert.Equal(t, "x", d.ProductTitle)
	assert.Equal(t, "x", d.Total)
	assert.Equal(t, "x", d.StatusNote)
	assert.Equal(t, "x", d.TrackingNumber)
	assert.Equal(t, "x", d.ShopName)
	assert.Equal(t, "x", d.OrderURL)
}
