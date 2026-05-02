package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ashep/simshop/internal/resend"
)

// notifyStatuses lists the order statuses whose templates are required at
// startup when Resend is enabled. Kept in sync with internal/resend's
// notifyStatuses; duplicating here avoids importing internal state and gives
// the validator a single source of "what we promise the customer."
var notifyStatuses = []string{
	"paid", "shipped", "delivered", "refund_requested", "refunded",
}

// validateEmailTemplates fails if `en.md` is missing for any of the
// notify-on statuses. Other languages are optional — Render will fall back
// to English at notify time. Reports every missing path in one error so the
// operator can fix the deployment in a single pass.
func validateEmailTemplates(store *resend.TemplateStore) error {
	if store == nil {
		return errors.New("email template store is nil")
	}
	all := store.All()
	var missing []string
	for _, st := range notifyStatuses {
		byLang, ok := all[st]
		if !ok {
			missing = append(missing, fmt.Sprintf("emails/%s (directory)", st))
			continue
		}
		if _, ok := byLang["en"]; !ok {
			missing = append(missing, fmt.Sprintf("emails/%s/en.md", st))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required email templates: %s", strings.Join(missing, ", "))
	}
	return nil
}
