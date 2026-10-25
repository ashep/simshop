package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/ashep/simshop/internal/order"
)

// tuiRefreshInterval is how often the order list is reloaded in the background.
const tuiRefreshInterval = 30 * time.Second

// pickerStatuses is the order in which operator targets are offered by the status picker.
var pickerStatuses = []string{
	"processing", "shipped", "delivered", "refund_requested", "returned", "refunded", "cancelled",
}

func newTUICmd(o *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive order browser",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI(cmd.Context(), o)
		},
	}
}

func runTUI(ctx context.Context, o *globalOpts) error {
	shop, err := o.selectShop()
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m := newTUIModel(ctx, shop, NewClient(shop.URL, shop.APIKey))
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}

type tuiMode int

const (
	modeList tuiMode = iota
	modeDetail
	modePicker
	modeInput
	modeSearch
)

type ordersMsg struct {
	orders []Order
	err    error
	at     time.Time
}

type statusMsg struct {
	id     string
	status string
	err    error
}

type tickMsg time.Time

// pendingChange is a status change assembled through the picker and prompts:
// shipped asks for a tracking number first, every change then asks for an
// optional note, and Enter on the note submits.
type pendingChange struct {
	order       Order
	status      string
	tracking    string
	askTracking bool
}

type tuiModel struct {
	ctx    context.Context
	shop   *Shop
	client *Client

	width, height int
	mode          tuiMode
	prevMode      tuiMode // mode to return to after a picker/prompt closes

	all      []Order // as loaded, newest first
	visible  []Order // after the status scope and search filter
	cursor   int
	offset   int
	showAll  bool
	search   string
	loading  bool
	loadedAt time.Time

	flash      string
	flashError bool

	detail      viewport.Model
	input       textinput.Model
	pickerItems []string
	pickerIdx   int
	change      *pendingChange
}

func newTUIModel(ctx context.Context, shop *Shop, client *Client) *tuiModel {
	ti := textinput.New()
	ti.CharLimit = 200
	return &tuiModel{
		ctx:    ctx,
		shop:   shop,
		client: client,
		input:  ti,
		detail: viewport.New(0, 0),
	}
}

func (m *tuiModel) Init() tea.Cmd {
	m.loading = true
	return tea.Batch(m.load(), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(tuiRefreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// load fetches orders for the current scope (active statuses or everything).
func (m *tuiModel) load() tea.Cmd {
	statuses := activeStatuses
	if m.showAll {
		statuses = nil
	}
	return func() tea.Msg {
		orders, err := m.client.ListOrders(m.ctx, statuses)
		return ordersMsg{orders: orders, err: err, at: time.Now()}
	}
}

func (m *tuiModel) setStatus(c pendingChange, note string) tea.Cmd {
	return func() tea.Msg {
		st, err := m.client.SetStatus(m.ctx, c.order.ID, c.status, c.tracking, note)
		return statusMsg{id: c.order.ID, status: st, err: err}
	}
}

func (m *tuiModel) selected() (Order, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return Order{}, false
	}
	return m.visible[m.cursor], true
}

// applyFilter rebuilds the visible list, keeping the cursor on the same order when possible.
func (m *tuiModel) applyFilter() {
	selID := ""
	if o, ok := m.selected(); ok {
		selID = o.ID
	}
	m.visible = filterOrders(m.all, m.search)
	m.cursor = 0
	for i, o := range m.visible {
		if o.ID == selID {
			m.cursor = i
			break
		}
	}
	m.clampScroll()
}

// filterOrders returns orders whose id, customer, contacts, city, product or tracking
// number contain every whitespace-separated term of q (case-insensitive).
func filterOrders(orders []Order, q string) []Order {
	terms := strings.Fields(strings.ToLower(q))
	if len(terms) == 0 {
		return orders
	}
	out := make([]Order, 0, len(orders))
	for _, o := range orders {
		hay := strings.ToLower(strings.Join([]string{
			o.ID, o.Status, o.FirstName, o.LastName, o.Email, o.Phone, o.City, o.Address,
			o.ProductID, deref(o.TrackingNumber), deref(o.PromoCode),
		}, " "))
		match := true
		for _, t := range terms {
			if !strings.Contains(hay, t) {
				match = false
				break
			}
		}
		if match {
			out = append(out, o)
		}
	}
	return out
}

// allowedTargets lists the statuses an operator may move an order in current to.
func allowedTargets(current string) []string {
	var out []string
	for _, s := range pickerStatuses {
		if order.ShouldApplyOperatorTransition(current, s) {
			out = append(out, s)
		}
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (m *tuiModel) listHeight() int {
	h := m.height - 5 // title, counters, column header, flash, key hints
	if h < 1 {
		h = 1
	}
	return h - m.panelHeight()
}

func (m *tuiModel) clampScroll() {
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *tuiModel) setFlash(s string, isErr bool) {
	m.flash, m.flashError = s, isErr
}

func (m *tuiModel) openDetail() {
	o, ok := m.selected()
	if !ok {
		return
	}
	m.mode = modeDetail
	m.detail.Width = m.width
	m.detail.Height = m.height - 4 - m.panelHeight()
	m.detail.SetContent(renderDetail(o, m.width))
	m.detail.GotoTop()
}

func (m *tuiModel) refreshDetail() {
	if m.mode != modeDetail && m.prevMode != modeDetail {
		return
	}
	if o, ok := m.selected(); ok {
		m.detail.SetContent(renderDetail(o, m.width))
	}
}

// openPicker opens the status picker with the targets allowed for the selected order.
func (m *tuiModel) openPicker() {
	o, ok := m.selected()
	if !ok {
		return
	}
	targets := allowedTargets(o.Status)
	if len(targets) == 0 {
		m.setFlash(fmt.Sprintf("no status changes allowed from %s", o.Status), true)
		return
	}
	m.prevMode = m.mode
	m.pickerItems = targets
	m.pickerIdx = 0
	m.mode = modePicker
}

func (m *tuiModel) startPrompt() {
	m.mode = modeInput
	m.input.Reset()
	if m.change.askTracking {
		m.input.Placeholder = "Nova Poshta waybill number"
		m.input.Prompt = "tracking › "
		m.input.SetValue(deref(m.change.order.TrackingNumber))
	} else {
		m.input.Placeholder = "optional, Enter to confirm"
		m.input.Prompt = "note › "
	}
	m.input.CursorEnd()
	m.input.Focus()
}

func (m *tuiModel) closePanel() {
	m.mode = m.prevMode
	m.change = nil
	m.input.Blur()
	m.clampScroll()
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.detail.Width = m.width
		m.detail.Height = m.height - 4 - m.panelHeight()
		m.refreshDetail()
		m.clampScroll()
		return m, nil

	case tickMsg:
		if m.loading {
			return m, tickCmd()
		}
		m.loading = true
		return m, tea.Batch(m.load(), tickCmd())

	case ordersMsg:
		m.loading = false
		if msg.err != nil {
			m.setFlash("load failed: "+msg.err.Error(), true)
			return m, nil
		}
		sort.SliceStable(msg.orders, func(i, j int) bool { return msg.orders[i].CreatedAt > msg.orders[j].CreatedAt })
		m.all = msg.orders
		m.loadedAt = msg.at
		m.applyFilter()
		m.refreshDetail()
		return m, nil

	case statusMsg:
		if msg.err != nil {
			m.setFlash(fmt.Sprintf("%s: %v", shortID(msg.id), msg.err), true)
			return m, nil
		}
		m.setFlash(fmt.Sprintf("%s → %s", shortID(msg.id), msg.status), false)
		m.loading = true
		return m, m.load()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *tuiModel) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.mode {
	case modeSearch:
		return m.handleSearchKey(k)
	case modeInput:
		return m.handleInputKey(k)
	case modePicker:
		return m.handlePickerKey(k)
	}

	// Keys shared by the list and the detail view.
	switch k.String() {
	case "q":
		if m.mode == modeDetail {
			m.mode = modeList
			return m, nil
		}
		return m, tea.Quit
	case "r":
		m.loading = true
		m.setFlash("", false)
		return m, m.load()
	case "a":
		m.showAll = !m.showAll
		m.loading = true
		return m, m.load()
	case "s":
		m.openPicker()
		return m, nil
	case "y":
		if o, ok := m.selected(); ok {
			termenv.Copy(o.ID)
			m.setFlash("copied "+o.ID, false)
		}
		return m, nil
	}

	if m.mode == modeDetail {
		switch k.String() {
		case "esc", "backspace", "h", "left":
			m.mode = modeList
			return m, nil
		case "J", "ctrl+n":
			m.moveCursor(1)
			m.openDetail()
			return m, nil
		case "K", "ctrl+p":
			m.moveCursor(-1)
			m.openDetail()
			return m, nil
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(k)
		return m, cmd
	}

	switch k.String() {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "ctrl+b":
		m.moveCursor(-m.listHeight())
	case "pgdown", "ctrl+f", " ":
		m.moveCursor(m.listHeight())
	case "home", "g":
		m.moveCursor(-len(m.visible))
	case "end", "G":
		m.moveCursor(len(m.visible))
	case "enter", "l", "right":
		m.openDetail()
	case "/":
		m.mode = modeSearch
		m.input.Reset()
		m.input.Prompt = "/"
		m.input.Placeholder = "name, phone, city, id, tracking…"
		m.input.SetValue(m.search)
		m.input.CursorEnd()
		m.input.Focus()
	case "esc":
		if m.search != "" {
			m.search = ""
			m.applyFilter()
		}
	}
	return m, nil
}

func (m *tuiModel) moveCursor(delta int) {
	m.cursor += delta
	m.clampScroll()
}

func (m *tuiModel) handleSearchKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.search = ""
		m.mode = modeList
		m.input.Blur()
		m.applyFilter()
		return m, nil
	case "enter":
		m.mode = modeList
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	m.search = m.input.Value()
	m.applyFilter()
	return m, cmd
}

func (m *tuiModel) handlePickerKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "q":
		m.closePanel()
	case "up", "k":
		if m.pickerIdx > 0 {
			m.pickerIdx--
		}
	case "down", "j":
		if m.pickerIdx < len(m.pickerItems)-1 {
			m.pickerIdx++
		}
	case "enter":
		o, ok := m.selected()
		if !ok {
			m.closePanel()
			return m, nil
		}
		target := m.pickerItems[m.pickerIdx]
		m.change = &pendingChange{order: o, status: target, askTracking: target == "shipped"}
		m.startPrompt()
	}
	return m, nil
}

func (m *tuiModel) handleInputKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.closePanel()
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if m.change.askTracking {
			if val == "" {
				m.setFlash("tracking number is required for shipped", true)
				return m, nil
			}
			m.change.tracking = val
			m.change.askTracking = false
			m.startPrompt()
			return m, nil
		}
		c := *m.change
		m.closePanel()
		m.setFlash(fmt.Sprintf("%s → %s…", shortID(c.order.ID), c.status), false)
		return m, m.setStatus(c, val)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}
