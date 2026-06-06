package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// formatPrice renders integer minor units as a decimal amount with uppercased currency.
func formatPrice(minor int, currency string) string {
	frac := minor % 100
	if frac < 0 {
		frac = -frac
	}
	return fmt.Sprintf("%d.%02d %s", minor/100, frac, strings.ToUpper(currency))
}

// RenderOrders writes orders as an aligned table.
func RenderOrders(w io.Writer, orders []Order) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tSTATUS\tCREATED\tCOUNTRY\tEMAIL\tTOTAL\tPRODUCT")
	for _, o := range orders {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			o.ID, o.Status, o.CreatedAt, strings.ToUpper(o.Country), o.Email,
			formatPrice(o.Price, o.Currency), o.ProductID)
	}
	return tw.Flush()
}

// RenderOrderDetail writes a single order's full record.
func RenderOrderDetail(w io.Writer, o *Order) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	p := func(k, v string) { _, _ = fmt.Fprintf(tw, "%s:\t%s\n", k, v) }
	p("ID", o.ID)
	p("Status", o.Status)
	p("Product", o.ProductID)
	p("Customer", strings.TrimSpace(o.FirstName+" "+o.LastName))
	p("Email", o.Email)
	p("Phone", o.Phone)
	p("Country", strings.ToUpper(o.Country))
	p("City", o.City)
	p("Address", o.Address)
	p("Total", formatPrice(o.Price, o.Currency))
	if o.TrackingNumber != nil && *o.TrackingNumber != "" {
		p("Tracking", *o.TrackingNumber)
	}
	p("Created", o.CreatedAt)
	p("Updated", o.UpdatedAt)
	if err := tw.Flush(); err != nil {
		return err
	}

	if len(o.Attrs) > 0 {
		_, _ = fmt.Fprintln(w, "Attributes:")
		for _, a := range o.Attrs {
			_, _ = fmt.Fprintf(w, "  - %s: %s\n", a.Name, a.Value)
		}
	}
	if len(o.History) > 0 {
		_, _ = fmt.Fprintln(w, "History:")
		for _, h := range o.History {
			_, _ = fmt.Fprintf(w, "  - %s  %s\n", h.CreatedAt, h.Status)
		}
	}
	return nil
}

// RenderShops writes configured shops as a table; api keys are masked.
func RenderShops(w io.Writer, cfg *Config) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tURL\tAPI_KEY\tDEFAULT")
	for _, s := range cfg.Shops {
		def := ""
		if s.Name == cfg.DefaultName() {
			def = "(default)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Name, s.URL, "<hidden>", def)
	}
	return tw.Flush()
}

// RenderJSON writes v as indented JSON.
func RenderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
