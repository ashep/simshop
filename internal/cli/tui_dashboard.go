package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// statsPeriods are the selectable dashboard periods in days; `p` cycles through them.
var statsPeriods = []int{7, 14, 30}

const (
	dashMaxChartHeight = 8
	dashMaxSlot        = 8 // widest column (bar + gap) per day
)

type statsMsg struct {
	days  int
	stats *Stats
	err   error
	at    time.Time
}

// openDashboard switches to the dashboard screen and loads its data.
func (m *tuiModel) openDashboard() tea.Cmd {
	m.screen = screenDashboard
	m.setFlash("", false)
	return m.loadStats()
}

func (m *tuiModel) loadStats() tea.Cmd {
	m.statsLoading = true
	days := m.statsDays
	return func() tea.Msg {
		st, err := m.client.GetStats(m.ctx, days)
		return statsMsg{days: days, stats: st, err: err, at: time.Now()}
	}
}

func (m *tuiModel) handleDashboardKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "tab":
		m.screen = screenOrders
		m.setFlash("", false)
	case "r":
		m.setFlash("", false)
		return m, m.loadStats()
	case "p":
		for i, d := range statsPeriods {
			if d == m.statsDays {
				m.statsDays = statsPeriods[(i+1)%len(statsPeriods)]
				break
			}
		}
		m.stats, m.statsErr = nil, nil
		m.setFlash("", false)
		return m, m.loadStats()
	}
	return m, nil
}

func (m *tuiModel) viewDashboard() string {
	var b strings.Builder
	b.WriteString(m.viewDashboardTitle())
	b.WriteString("\n")

	chartH := min(max(m.height-23, 3), dashMaxChartHeight)
	rows := max(m.height-chartH-13, 3)

	switch {
	case errors.Is(m.statsErr, ErrStatsNotConfigured):
		b.WriteString("\n" + styleDim.Render(" Stats are not configured on the server") + "\n")
	case m.stats == nil:
		msg := " no data"
		if m.statsLoading {
			msg = " loading…"
		}
		b.WriteString("\n" + styleDim.Render(msg) + "\n")
	default:
		days := m.stats.Days
		total := 0
		for _, d := range days {
			total += d.Visits
		}
		avg := 0
		if len(days) > 0 {
			avg = (total + len(days)/2) / len(days)
		}
		b.WriteString("\n " + styleHeader.Render(fmt.Sprintf("Visits, last %d days", m.statsDays)) +
			styleDim.Render(fmt.Sprintf(" · total %s · avg %s/day", thousands(total), thousands(avg))) + "\n\n")
		for _, line := range strings.Split(renderBars(days, m.width-2, chartH), "\n") {
			b.WriteString(" " + line + "\n")
		}
		b.WriteString("\n " + styleHeader.Render("Top products") + "\n")
		ps := m.stats.TopProducts
		if len(ps) > rows {
			ps = ps[:rows]
		}
		for _, line := range strings.Split(renderTopProducts(ps, m.width-2), "\n") {
			b.WriteString(" " + line + "\n")
		}
	}

	b.WriteString(m.viewFlash())
	b.WriteString("\n")
	hints := [][2]string{{"p", "period"}, {"r", "refresh"}, {"tab", "orders"}, {"q", "quit"}}
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = styleKey.Render("<"+h[0]+">") + " " + styleDim.Render(h[1])
	}
	b.WriteString(strings.Join(parts, "  "))
	return lipgloss.NewStyle().MaxWidth(m.width).Render(b.String()) // narrow terminals clip instead of wrapping
}

func (m *tuiModel) viewDashboardTitle() string {
	left := styleTitle.Render("simshop") + " " +
		lipgloss.NewStyle().Bold(true).Render(m.shop.Name) + "  " +
		styleScope.Render(fmt.Sprintf("DASHBOARD %dd", m.statsDays))
	right := ""
	switch {
	case m.statsLoading:
		right = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("⟳ loading")
	case !m.statsAt.IsZero():
		right = styleDim.Render("updated " + m.statsAt.Format("15:04:05"))
	}
	gap := max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// thousands formats n with a space as the thousands separator.
func thousands(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	out = append([]string{s}, out...)
	r := strings.Join(out, " ")
	if neg {
		r = "-" + r
	}
	return r
}

// centerIn centers s in w cells (truncating if too long).
func centerIn(s string, w int) string {
	s = runewidth.Truncate(s, w, "")
	left := (w - runewidth.StringWidth(s)) / 2
	return fit(strings.Repeat(" ", left)+s, w)
}

// compactCount shortens big numbers (1200 -> 1.2k) when they would not fit in w cells.
func compactCount(n, w int) string {
	s := strconv.Itoa(n)
	if len(s) <= w {
		return s
	}
	if n >= 1000 && n < 10000 {
		if d := fmt.Sprintf("%.1fk", float64(n)/1000); len(d) <= w {
			return d
		}
	}
	if k := fmt.Sprintf("%dk", n/1000); n >= 1000 && len(k) <= w {
		return k
	}
	return "" // no honest form fits; leave the cell empty rather than misreport
}

// renderBars draws a vertical bar chart of daily visits that is at most width cells wide:
// a y-axis labelled with the maximum, height rows of bars, an x-axis, date labels
// (weekday + day of month when columns are wide enough, day of month otherwise) and, for
// wide columns, a row with the per-day values.
func renderBars(days []StatsDay, width, height int) string {
	if len(days) == 0 || height < 1 {
		return ""
	}
	maxV := 0
	for _, d := range days {
		maxV = max(maxV, d.Visits)
	}
	axisLabel := thousands(maxV)
	axisW := len(axisLabel) + 1 // label + space; then the axis glyph
	chartW := width - axisW - 1
	if chartW < 1 {
		return ""
	}
	slot := min(chartW/len(days), dashMaxSlot)
	if slot < 1 {
		slot = 1
		days = days[len(days)-chartW:]
	}
	barW := slot
	if slot >= 2 {
		barW = slot - 1
	}
	gap := strings.Repeat(" ", slot-barW)

	heights := make([]int, len(days))
	for i, d := range days {
		if maxV > 0 && d.Visits > 0 {
			heights[i] = max((d.Visits*height+maxV/2)/maxV, 1)
		}
	}

	var lines []string
	for r := height; r >= 1; r-- {
		var b strings.Builder
		if r == height {
			b.WriteString(styleDim.Render(fitLeft(axisLabel, axisW-1)) + " " + styleFaint.Render("┤"))
		} else {
			b.WriteString(strings.Repeat(" ", axisW) + styleFaint.Render("│"))
		}
		for _, h := range heights {
			if h >= r {
				b.WriteString(lipgloss.NewStyle().Foreground(colAccent).Render(strings.Repeat("█", barW)) + gap)
			} else {
				b.WriteString(strings.Repeat(" ", slot))
			}
		}
		lines = append(lines, b.String())
	}
	pad := strings.Repeat(" ", axisW)
	lines = append(lines, pad+styleFaint.Render("└"+strings.Repeat("─", len(days)*slot)))

	label := func(texts []string, every int) string {
		var b strings.Builder
		b.WriteString(pad + " ")
		for i, t := range texts {
			if i%every == 0 {
				b.WriteString(styleDim.Render(centerIn(t, barW)) + gap)
			} else {
				b.WriteString(strings.Repeat(" ", slot))
			}
		}
		return strings.TrimRight(b.String(), " ")
	}
	dom := make([]string, len(days))
	wd := make([]string, len(days))
	for i, d := range days {
		dom[i], wd[i] = d.Date[max(len(d.Date)-2, 0):], ""
		if t, err := time.Parse("2006-01-02", d.Date); err == nil {
			dom[i] = fmt.Sprintf("%02d", t.Day())
			wd[i] = t.Weekday().String()[:2]
		}
	}
	if slot >= 3 {
		lines = append(lines, label(wd, 1), label(dom, 1))
	} else {
		// Narrow columns: write the full 2-char day at the slot start, one label per k slots.
		every := (3 + slot - 1) / slot
		var b strings.Builder
		for i, t := range dom {
			if i%every == 0 {
				b.WriteString(fit(t, slot*every))
			}
		}
		lines = append(lines, pad+" "+styleDim.Render(strings.TrimRight(runewidth.Truncate(b.String(), len(days)*slot, ""), " ")))
	}
	if slot >= 4 {
		vals := make([]string, len(days))
		for i, d := range days {
			vals[i] = compactCount(d.Visits, barW)
		}
		lines = append(lines, label(vals, 1))
	}
	return strings.Join(lines, "\n")
}

// renderTopProducts renders the ranked product table at most width cells wide.
func renderTopProducts(ps []StatsProduct, width int) string {
	const rankW, viewsW = 3, 7
	nameW := max(width-rankW-viewsW-2, 4)
	var lines []string
	lines = append(lines, styleHeader.Render(fitLeft("#", rankW)+" "+fit("Product", nameW)+" "+fitLeft("Views", viewsW)))
	if len(ps) == 0 {
		lines = append(lines, styleDim.Render("no product views yet"))
	}
	for i, p := range ps {
		name := styleValue.Render(fit(p.Title, nameW))
		if p.Title == "" {
			name = styleDim.Render(fit(p.ID, nameW))
		}
		lines = append(lines, styleDim.Render(fitLeft(strconv.Itoa(i+1), rankW))+" "+name+" "+
			styleMoney.Render(fitLeft(thousands(p.Views), viewsW)))
	}
	return strings.Join(lines, "\n")
}
