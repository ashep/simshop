package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	colAccent = lipgloss.Color("39")
	colDim    = lipgloss.Color("244")
	colFaint  = lipgloss.Color("238")
	colSelBg  = lipgloss.Color("237")
	colError  = lipgloss.Color("196")
	colOK     = lipgloss.Color("42")

	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("25")).Padding(0, 1)
	styleScope   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("214")).Padding(0, 1)
	styleDim     = lipgloss.NewStyle().Foreground(colDim)
	styleFaint   = lipgloss.NewStyle().Foreground(colFaint)
	styleHeader  = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styleKey     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	styleLabel   = lipgloss.NewStyle().Foreground(colDim)
	styleValue   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleSection = lipgloss.NewStyle().Bold(true).Foreground(colAccent).MarginTop(1)
	styleMoney   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	stylePanel   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(0, 1)
)

// statusColors maps each order status to its display color: grey for pre-payment,
// yellow for payment in flight, bright green for "paid" (needs action), then the
// fulfillment colors, orange/pink for the refund path, and grey again for the closed
// states (refunded a shade brighter than cancelled).
var statusColors = map[string]lipgloss.Color{
	"new":                "245",
	"awaiting_payment":   "245",
	"payment_processing": "220",
	"payment_hold":       "214",
	"paid":               "46",
	"processing":         "45",
	"shipped":            "141",
	"delivered":          "35",
	"refund_requested":   "208",
	"returned":           "168",
	"refunded":           "250",
	"cancelled":          "244",
}

func statusStyle(s string) lipgloss.Style {
	st := lipgloss.NewStyle().Foreground(statusColors[s])
	if s == "paid" || s == "refund_requested" {
		st = st.Bold(true)
	}
	return st
}

// fit truncates or pads s to exactly w terminal cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = runewidth.Truncate(s, w, "…")
	return runewidth.FillRight(s, w)
}

// fitLeft right-aligns s in w terminal cells.
func fitLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = runewidth.Truncate(s, w, "…")
	return runewidth.FillLeft(s, w)
}

func customerName(o Order) string {
	return strings.TrimSpace(o.FirstName + " " + o.LastName)
}

func shortTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	t = t.Local()
	if t.Year() != time.Now().Year() {
		return t.Format("2006-01-02")
	}
	return t.Format("Jan 02 15:04")
}

func (m *tuiModel) View() string {
	if m.width == 0 {
		return "loading…"
	}
	if m.screen == screenDashboard {
		return m.viewDashboard()
	}
	var b strings.Builder
	b.WriteString(m.viewTitle())
	b.WriteString("\n")
	b.WriteString(m.viewCounters())
	b.WriteString("\n")
	if m.mode == modeDetail || (m.prevMode == modeDetail && (m.mode == modePicker || m.mode == modeInput)) {
		m.detail.Height = max(m.height-4-m.panelHeight(), 1)
		b.WriteString(m.detail.View())
	} else {
		b.WriteString(m.viewTable())
	}
	b.WriteString("\n")
	if p := m.viewPanel(); p != "" {
		b.WriteString(p)
		b.WriteString("\n")
	}
	b.WriteString(m.viewFlash())
	b.WriteString("\n")
	b.WriteString(m.viewKeys())
	return b.String()
}

func (m *tuiModel) viewTitle() string {
	scope := "ACTIVE"
	if m.showAll {
		scope = "ALL"
	}
	left := styleTitle.Render("simshop") + " " +
		lipgloss.NewStyle().Bold(true).Render(m.shop.Name) + " " +
		styleDim.Render(m.shop.URL) + "  " + styleScope.Render(scope)
	if m.search != "" || m.mode == modeSearch {
		left += "  " + styleKey.Render("/") + lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Render(m.search)
	}
	right := ""
	switch {
	case m.loading:
		right = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("⟳ loading")
	case !m.loadedAt.IsZero():
		right = styleDim.Render("updated " + m.loadedAt.Format("15:04:05"))
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// viewCounters renders a colored per-status count of the loaded orders.
func (m *tuiModel) viewCounters() string {
	counts := map[string]int{}
	for _, o := range m.all {
		counts[o.Status]++
	}
	order := []string{
		"paid", "processing", "shipped", "refund_requested", "payment_hold", "payment_processing",
		"awaiting_payment", "new", "delivered", "returned", "refunded", "cancelled",
	}
	parts := []string{styleDim.Render(fmt.Sprintf("%d/%d orders", len(m.visible), len(m.all)))}
	for _, s := range order {
		if n := counts[s]; n > 0 {
			parts = append(parts, statusStyle(s).Render(fmt.Sprintf("● %s %d", s, n)))
		}
	}
	return strings.Join(parts, styleFaint.Render("  "))
}

type column struct {
	title string
	width int
	right bool
}

func (m *tuiModel) columns() []column {
	cols := []column{
		{title: "ID", width: 13},
		{title: "STATUS", width: 20},
		{title: "CREATED", width: 12},
		{title: "CUSTOMER", width: 0},
		{title: "PHONE", width: 14},
		{title: "CITY", width: 0},
		{title: "PRODUCT", width: 0},
		{title: "TOTAL", width: 12, right: true},
	}
	fixed, flex := 0, 0
	for _, c := range cols {
		fixed += c.width
		if c.width == 0 {
			flex++
		}
	}
	free := m.width - fixed - (len(cols) - 1) - 2 // column gaps and side padding
	if free < flex*6 {
		free = flex * 6
	}
	for i := range cols {
		if cols[i].width == 0 {
			cols[i].width = free / flex
		}
	}
	return cols
}

func (m *tuiModel) viewTable() string {
	cols := m.columns()
	var b strings.Builder

	hdr := make([]string, len(cols))
	for i, c := range cols {
		if c.right {
			hdr[i] = fitLeft(c.title, c.width)
		} else {
			hdr[i] = fit(c.title, c.width)
		}
	}
	b.WriteString(styleHeader.Render(" " + strings.Join(hdr, " ") + " "))

	h := m.listHeight()
	if len(m.visible) == 0 {
		msg := "no orders"
		if m.search != "" {
			msg = "nothing matches /" + m.search
		}
		b.WriteString("\n" + styleDim.Render(" "+msg))
		h--
		b.WriteString(strings.Repeat("\n", max(h, 0)))
		return b.String()
	}

	for i := m.offset; i < m.offset+h; i++ {
		b.WriteString("\n")
		if i >= len(m.visible) {
			continue
		}
		b.WriteString(m.viewRow(m.visible[i], cols, i == m.cursor))
	}
	return b.String()
}

func (m *tuiModel) viewRow(o Order, cols []column, sel bool) string {
	cells := []struct {
		text  string
		style lipgloss.Style
	}{
		{shortID(o.ID), styleDim},
		{"● " + o.Status, statusStyle(o.Status)},
		{shortTime(o.CreatedAt), styleDim},
		{customerName(o), styleValue},
		{o.Phone, styleValue},
		{o.City, styleValue},
		{o.ProductID, styleValue},
		{formatPrice(o.Price, o.Currency), styleMoney},
	}

	pad := lipgloss.NewStyle()
	if sel {
		pad = pad.Background(colSelBg)
	}
	var b strings.Builder
	b.WriteString(pad.Render(" "))
	for i, c := range cells {
		text := fit(c.text, cols[i].width)
		if cols[i].right {
			text = fitLeft(c.text, cols[i].width)
		}
		st := c.style
		if sel {
			st = st.Background(colSelBg).Bold(true)
		}
		b.WriteString(st.Render(text))
		if i < len(cells)-1 {
			b.WriteString(pad.Render(" "))
		}
	}
	b.WriteString(pad.Render(" "))
	return b.String()
}

// panelHeight is the number of lines the picker/prompt panel occupies.
func (m *tuiModel) panelHeight() int {
	switch m.mode {
	case modePicker:
		return len(m.pickerItems) + 3 // border + title
	case modeInput:
		return 4
	case modeSearch:
		return 1
	}
	return 0
}

func (m *tuiModel) viewPanel() string {
	switch m.mode {
	case modeSearch:
		return m.input.View()
	case modePicker:
		o, _ := m.selected()
		lines := []string{
			"Move " + styleDim.Render(shortID(o.ID)) + " from " + statusStyle(o.Status).Render(o.Status) + " to:",
		}
		for i, s := range m.pickerItems {
			cursor := "  "
			if i == m.pickerIdx {
				cursor = styleKey.Render("▸ ")
			}
			lines = append(lines, cursor+statusStyle(s).Render(s))
		}
		return stylePanel.Width(m.width - 2).Render(strings.Join(lines, "\n"))
	case modeInput:
		c := m.change
		title := "Set " + styleDim.Render(shortID(c.order.ID)) + " " + statusStyle(c.order.Status).Render(c.order.Status) +
			" → " + statusStyle(c.status).Render(c.status)
		if c.tracking != "" {
			title += styleDim.Render("  tracking " + c.tracking)
		}
		st := stylePanel
		if c.status == "cancelled" || c.status == "refunded" {
			st = st.BorderForeground(colError)
		}
		return st.Width(m.width - 2).Render(title + "\n" + m.input.View())
	}
	return ""
}

func (m *tuiModel) viewFlash() string {
	if m.flash == "" {
		return ""
	}
	st := lipgloss.NewStyle().Foreground(colOK)
	icon := "✓ "
	if m.flashError {
		st = lipgloss.NewStyle().Foreground(colError).Bold(true)
		icon = "✗ "
	}
	return st.Render(fit(icon+m.flash, m.width))
}

func (m *tuiModel) viewKeys() string {
	type hint struct{ key, desc string }
	var hints []hint
	switch m.mode {
	case modeSearch:
		hints = []hint{{"enter", "apply"}, {"esc", "clear"}}
	case modePicker:
		hints = []hint{{"↑↓", "choose"}, {"enter", "select"}, {"esc", "cancel"}}
	case modeInput:
		hints = []hint{{"enter", "confirm"}, {"esc", "cancel"}}
	case modeDetail:
		hints = []hint{
			{"esc", "back"}, {"↑↓", "scroll"}, {"J/K", "next/prev"}, {"s", "status…"}, {"y", "copy id"}, {"r", "refresh"},
			{"tab", "dashboard"},
		}
	default:
		hints = []hint{
			{"enter", "open"}, {"/", "search"}, {"s", "status…"}, {"a", "active/all"}, {"y", "copy id"}, {"r", "refresh"},
			{"tab", "dashboard"}, {"q", "quit"},
		}
	}
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = styleKey.Render("<"+h.key+">") + " " + styleDim.Render(h.desc)
	}
	return strings.Join(parts, "  ")
}

// renderDetail renders the full order card shown in the detail view.
func renderDetail(o Order, width int) string {
	var b strings.Builder
	kv := func(k, v string) {
		if v == "" {
			return
		}
		b.WriteString(styleLabel.Render(fit(k, 12)) + " " + styleValue.Render(v) + "\n")
	}

	b.WriteString(" " + statusStyle(o.Status).Reverse(true).Bold(true).Padding(0, 1).Render(o.Status) + "  " +
		lipgloss.NewStyle().Bold(true).Render(o.ID) + "  " + styleMoney.Render(formatPrice(o.Price, o.Currency)) + "\n")

	b.WriteString(styleSection.Render("Customer") + "\n")
	name := strings.TrimSpace(o.FirstName + " " + deref(o.MiddleName) + " " + o.LastName)
	kv("Name", strings.Join(strings.Fields(name), " "))
	kv("Phone", o.Phone)
	kv("Email", o.Email)
	kv("Language", o.Lang)

	b.WriteString(styleSection.Render("Delivery") + "\n")
	kv("Country", strings.ToUpper(o.Country))
	kv("City", o.City)
	kv("Address", o.Address)
	kv("Tracking", deref(o.TrackingNumber))

	b.WriteString(styleSection.Render("Order") + "\n")
	kv("Product", o.ProductID)
	for _, a := range o.Attrs {
		v := a.Value
		if a.Price != 0 {
			v += styleDim.Render("  +" + formatPrice(a.Price, o.Currency))
		}
		kv("  "+a.Name, v)
	}
	if o.PromoCode != nil && o.DiscountPercent != nil && o.Discount != nil {
		kv("Promo", fmt.Sprintf("%s (−%d%%, −%s)", *o.PromoCode, *o.DiscountPercent, formatPrice(*o.Discount, o.Currency)))
	}
	kv("Total", styleMoney.Render(formatPrice(o.Price, o.Currency)))
	kv("Created", formatTime(o.CreatedAt))
	kv("Updated", formatTime(o.UpdatedAt))

	if n := deref(o.CustomerNote); n != "" {
		b.WriteString(styleSection.Render("Customer note") + "\n")
		b.WriteString(lipgloss.NewStyle().Width(max(width-2, 20)).Foreground(lipgloss.Color("229")).Render(n) + "\n")
	}
	if n := deref(o.AdminNote); n != "" {
		b.WriteString(styleSection.Render("Admin note") + "\n")
		b.WriteString(lipgloss.NewStyle().Width(max(width-2, 20)).Render(n) + "\n")
	}

	if len(o.Invoices) > 0 {
		b.WriteString(styleSection.Render("Invoices") + "\n")
		for _, inv := range o.Invoices {
			b.WriteString(styleDim.Render(inv.Provider) + "  " + inv.ID + "  " +
				styleMoney.Render(formatPrice(inv.Amount, inv.Currency)) + "  " + styleDim.Render(inv.PageURL) + "\n")
		}
	}

	if len(o.History) > 0 {
		b.WriteString(styleSection.Render("History") + "\n")
		for _, h := range o.History {
			line := styleDim.Render(formatTime(h.CreatedAt)) + "  " + statusStyle(h.Status).Render(fit("● "+h.Status, 20))
			if n := deref(h.Note); n != "" {
				line += "  " + styleValue.Render(n)
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}
