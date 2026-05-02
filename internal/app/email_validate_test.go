package app

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/resend"
)

func TestValidateEmailTemplates(main *testing.T) {
	main.Run("AllStatusesPresent", func(t *testing.T) {
		store := buildStore(t, []string{"paid", "shipped", "delivered", "refund_requested", "refunded"})
		require.NoError(t, validateEmailTemplates(store))
	})

	main.Run("NilStoreErrors", func(t *testing.T) {
		err := validateEmailTemplates(nil)
		require.Error(t, err)
	})

	main.Run("MissingPaidErrors", func(t *testing.T) {
		store := buildStore(t, []string{"shipped", "delivered", "refund_requested", "refunded"})
		err := validateEmailTemplates(store)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "paid")
	})

	main.Run("MissingEnglishErrors", func(t *testing.T) {
		// status present, but only `uk` template — must require `en`.
		store, err := resend.LoadTemplates(makeDirOnlyUk(t))
		require.NoError(t, err)
		err = validateEmailTemplates(store)
		require.Error(t, err)
	})
}

// buildStore writes en.md for each status under a temp dir and loads it.
func buildStore(t *testing.T, statuses []string) *resend.TemplateStore {
	t.Helper()
	dir := t.TempDir()
	for _, s := range statuses {
		sd := dir + "/" + s
		require.NoError(t, os.MkdirAll(sd, 0755))
		require.NoError(t, os.WriteFile(sd+"/en.md", []byte("---\nsubject: x\n---\nbody"), 0644))
	}
	store, err := resend.LoadTemplates(dir)
	require.NoError(t, err)
	return store
}

// makeDirOnlyUk writes uk.md (no en.md) for every notify-on status so the
// validator can prove `en` is required.
func makeDirOnlyUk(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, s := range []string{"paid", "shipped", "delivered", "refund_requested", "refunded"} {
		sd := dir + "/" + s
		require.NoError(t, os.MkdirAll(sd, 0755))
		require.NoError(t, os.WriteFile(sd+"/uk.md", []byte("---\nsubject: x\n---\nbody"), 0644))
	}
	return dir
}
