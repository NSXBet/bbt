// Package bbt provides a pluggable Bubble Tea component that adds
// a modal todo/bead creation form and a list viewer to any Bubble Tea app.
// Persists via the `bd` CLI.
package bbt

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	lipgloss "charm.land/lipgloss/v2"
)

// ---------------------------------------------------------------------------
// Bead — maps to bd JSON output
// ---------------------------------------------------------------------------

type Bead struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Type      string `json:"issue_type"`
	Priority  int    `json:"priority"`
	CreatedBy string `json:"created_by"`
}

func statusIcon(s string) string {
	switch s {
	case "open":
		return "○"
	case "in_progress":
		return "◑"
	case "closed":
		return "●"
	default:
		return "?"
	}
}

func statusDisplay(s string) string {
	switch s {
	case "in_progress":
		return "in-progress"
	default:
		return s
	}
}

func nextStatus(s string) string {
	switch s {
	case "open":
		return "in_progress"
	case "in_progress":
		return "closed"
	case "closed":
		return "open"
	default:
		return "open"
	}
}

// ---------------------------------------------------------------------------
// bd CLI helpers
// ---------------------------------------------------------------------------

func bdRun(args ...string) ([]byte, error) {
	out, err := exec.Command("bd", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return out, nil
}

func bdCheck() error {
	if _, err := exec.LookPath("bd"); err != nil {
		return fmt.Errorf("bd not found in PATH")
	}
	_, err := bdRun("list", "--json", "--limit=1")
	return err
}

func bdList() ([]Bead, error) {
	out, err := bdRun("list", "--json")
	if err != nil {
		return nil, err
	}
	var beads []Bead
	if err := json.Unmarshal(out, &beads); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return beads, nil
}

func bdCreate(title string) (*Bead, error) {
	out, err := bdRun("create", title, "-t", "task", "--json")
	if err != nil {
		return nil, err
	}
	var b Bead
	if err := json.Unmarshal(out, &b); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return &b, nil
}

func bdSetStatus(id, status string) error {
	switch status {
	case "in_progress":
		_, err := bdRun("update", id, "--claim", "--json")
		return err
	case "closed":
		_, err := bdRun("close", id, "--json")
		return err
	case "open":
		_, err := bdRun("reopen", id, "--json")
		return err
	}
	return nil
}

func bdDelete(id string) error {
	_, err := bdRun("delete", id, "--force")
	return err
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

type Colors struct {
	Border     string
	Title      string
	StatusOpen string
	StatusWIP  string
	StatusDone string
	DimDark    string
	DimLight   string
	SelDark    string
	SelLight   string
}

func DefaultColors() Colors {
	return Colors{
		Border: "#7D56F4", Title: "#FF75B5",
		StatusOpen: "#888888", StatusWIP: "#FFAA00", StatusDone: "#00CC66",
		DimDark: "#555555", DimLight: "#AAAAAA",
		SelDark: "#3A3A5C", SelLight: "#E8E8FF",
	}
}

func (c Colors) withDefaults() Colors {
	d := DefaultColors()
	set := func(dst *string, def string) {
		if *dst == "" {
			*dst = def
		}
	}
	set(&c.Border, d.Border)
	set(&c.Title, d.Title)
	set(&c.StatusOpen, d.StatusOpen)
	set(&c.StatusWIP, d.StatusWIP)
	set(&c.StatusDone, d.StatusDone)
	set(&c.DimDark, d.DimDark)
	set(&c.DimLight, d.DimLight)
	set(&c.SelDark, d.SelDark)
	set(&c.SelLight, d.SelLight)
	return c
}

type Config struct {
	KeyMap      *KeyMap
	Colors      Colors
	Placeholder string
	ModalWidth  int
	CreateIcon  string
	ListIcon    string
}

func DefaultConfig() Config {
	return Config{
		Colors: DefaultColors(), Placeholder: "What needs to be done?",
		CreateIcon: "✦", ListIcon: "✦",
	}
}

// ---------------------------------------------------------------------------
// KeyMap
// ---------------------------------------------------------------------------

type KeyMap struct {
	OpenCreate key.Binding
	OpenList   key.Binding
	Confirm    key.Binding
	Cancel     key.Binding
	NextStatus key.Binding
	Delete     key.Binding
	MoveUp     key.Binding
	MoveDown   key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		OpenCreate: key.NewBinding(key.WithKeys("f7"), key.WithHelp("f7", "new bead")),
		OpenList:   key.NewBinding(key.WithKeys("f8"), key.WithHelp("f8", "list beads")),
		Confirm:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		Cancel:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
		NextStatus: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "cycle status")),
		Delete:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		MoveUp:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		MoveDown:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	}
}

// ---------------------------------------------------------------------------
// Functional options
// ---------------------------------------------------------------------------

type Option func(*Model)

func WithKeyMap(km KeyMap) Option     { return func(m *Model) { m.keys = km } }
func WithPlaceholder(s string) Option { return func(m *Model) { m.input.Placeholder = s } }

func WithColors(c Colors) Option {
	return func(m *Model) {
		m.colors = c.withDefaults()
		m.styles = buildStyles(m.colors, m.isDark)
	}
}

// WithDemoMode skips bd CLI entirely — beads are in-memory only.
// Useful for testing the TUI without a beads database.
func WithDemoMode() Option {
	return func(m *Model) {
		m.checked = true
		m.ready = true
		m.demoMode = true
	}
}

func WithConfig(cfg Config) Option {
	return func(m *Model) {
		if cfg.KeyMap != nil {
			m.keys = *cfg.KeyMap
		}
		m.colors = cfg.Colors.withDefaults()
		m.styles = buildStyles(m.colors, m.isDark)
		if cfg.Placeholder != "" {
			m.input.Placeholder = cfg.Placeholder
		}
		m.modalWidth = cfg.ModalWidth
		if cfg.CreateIcon != "" {
			m.createIcon = cfg.CreateIcon
		}
		if cfg.ListIcon != "" {
			m.listIcon = cfg.ListIcon
		}
	}
}

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

type styles struct {
	Modal       lipgloss.Style
	Title       lipgloss.Style
	SelectedRow lipgloss.Style
	StatusOpen  lipgloss.Style
	StatusWIP   lipgloss.Style
	StatusDone  lipgloss.Style
	Help        lipgloss.Style
	Dimmed      lipgloss.Style
	Error       lipgloss.Style
}

func buildStyles(c Colors, isDark bool) styles {
	var dim, sel color.Color
	if isDark {
		dim = lipgloss.Color(c.DimDark)
		sel = lipgloss.Color(c.SelDark)
	} else {
		dim = lipgloss.Color(c.DimLight)
		sel = lipgloss.Color(c.SelLight)
	}
	return styles{
		Modal: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(c.Border)).
			Padding(1, 2),
		Title: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Title)).
			Bold(true).MarginBottom(1),
		SelectedRow: lipgloss.NewStyle().Background(sel),
		StatusOpen:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.StatusOpen)),
		StatusWIP:   lipgloss.NewStyle().Foreground(lipgloss.Color(c.StatusWIP)),
		StatusDone:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.StatusDone)),
		Help:        lipgloss.NewStyle().Foreground(dim).MarginTop(1),
		Dimmed:      lipgloss.NewStyle().Foreground(dim),
		Error:       lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4444")).Bold(true),
	}
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// Public — host app can react to these
type BeadAddedMsg struct{ Bead Bead }
type BeadStatusChangedMsg struct{ Bead Bead }
type BeadDeletedMsg struct{ ID string }

// Internal
type beadsLoadedMsg struct{ beads []Bead }
type bdErrorMsg struct{ err string }
type bdCheckDoneMsg struct{ err error }
type beadCreatedMsg struct{ bead Bead }
type beadStatusUpdatedMsg struct {
	id     string
	status string
}
type beadDeletedInternalMsg struct{ id string }

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type mode int

const (
	modeInactive mode = iota
	modeCreate
	modeList
)

type Model struct {
	beads        []Bead
	mode         mode
	input        textinput.Model
	cursor       int
	createScroll int
	keys         KeyMap
	colors       Colors
	styles       styles
	width        int
	height       int
	isDark       bool
	modalWidth   int
	createIcon   string
	listIcon     string
	errMsg       string
	checked      bool
	ready        bool
	loading      bool
	demoMode     bool
}

func New(opts ...Option) Model {
	ti := textinput.New()
	ti.Placeholder = "What needs to be done?"
	ti.Prompt = "> "
	ti.SetWidth(40)
	ti.SetVirtualCursor(true)
	ti.CharLimit = 256

	c := DefaultColors()
	m := Model{
		keys: DefaultKeyMap(), input: ti, colors: c,
		isDark: true, styles: buildStyles(c, true),
		createIcon: "✦", listIcon: "✦",
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// Init checks if bd is available. Batch with your own Init commands.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		return bdCheckDoneMsg{err: bdCheck()}
	}
}

func (m Model) Active() bool  { return m.mode != modeInactive }
func (m Model) Beads() []Bead { return m.beads }

func (m *Model) SetSize(w, h int) { m.width = w; m.height = h }
func (m *Model) SetDark(isDark bool) {
	m.isDark = isDark
	m.styles = buildStyles(m.colors, isDark)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.BackgroundColorMsg:
		m.SetDark(msg.IsDark())

	case bdCheckDoneMsg:
		m.checked = true
		if msg.err != nil {
			m.errMsg = msg.err.Error()
		} else {
			m.ready = true
		}

	case beadsLoadedMsg:
		m.beads = msg.beads
		m.loading = false
		if m.cursor >= len(m.beads) {
			m.cursor = max(0, len(m.beads)-1)
		}

	case beadCreatedMsg:
		m.beads = append(m.beads, msg.bead)
		m.input.SetValue("")
		bead := msg.bead
		return m, func() tea.Msg { return BeadAddedMsg{Bead: bead} }

	case beadStatusUpdatedMsg:
		for i := range m.beads {
			if m.beads[i].ID == msg.id {
				m.beads[i].Status = msg.status
				bead := m.beads[i]
				return m, func() tea.Msg { return BeadStatusChangedMsg{Bead: bead} }
			}
		}

	case beadDeletedInternalMsg:
		for i := range m.beads {
			if m.beads[i].ID == msg.id {
				m.beads = append(m.beads[:i], m.beads[i+1:]...)
				if m.cursor >= len(m.beads) && m.cursor > 0 {
					m.cursor--
				}
				break
			}
		}
		id := msg.id
		return m, func() tea.Msg { return BeadDeletedMsg{ID: id} }

	case bdErrorMsg:
		m.errMsg = msg.err
		m.loading = false

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Forward non-key messages to textinput (cursor blink)
	if m.mode == modeCreate && m.ready {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.mode == modeInactive {
		switch {
		case key.Matches(msg, m.keys.OpenCreate):
			if !m.checked {
				return m, nil
			}
			m.mode = modeCreate
			m.errMsg = ""
			m.createScroll = 0
			if m.ready {
				m.input.SetValue("")
				if m.demoMode {
					m.loading = false
					cmd := m.input.Focus()
					return m, cmd
				}
				m.loading = true
				cmd := m.input.Focus()
				return m, tea.Batch(cmd, loadBeads())
			}
			return m, nil

		case key.Matches(msg, m.keys.OpenList):
			if !m.checked {
				return m, nil
			}
			m.mode = modeList
			m.errMsg = ""
			m.cursor = 0
			if m.ready {
				if m.demoMode {
					m.loading = false
					return m, nil
				}
				m.loading = true
				return m, loadBeads()
			}
			return m, nil
		}
		return m, nil
	}

	// Cancel from any modal
	if key.Matches(msg, m.keys.Cancel) {
		m.mode = modeInactive
		m.errMsg = ""
		m.input.Blur()
		return m, nil
	}

	switch m.mode {
	case modeCreate:
		return m.updateCreate(msg)
	case modeList:
		return m.updateList(msg)
	}
	return m, nil
}

func loadBeads() tea.Cmd {
	return func() tea.Msg {
		beads, err := bdList()
		if err != nil {
			return bdErrorMsg{err: err.Error()}
		}
		return beadsLoadedMsg{beads: beads}
	}
}

func (m Model) updateCreate(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if !m.ready {
		return m, nil
	}

	if key.Matches(msg, m.keys.Confirm) {
		title := strings.TrimSpace(m.input.Value())
		if title != "" {
			if m.demoMode {
				b := Bead{ID: fmt.Sprintf("demo-%d", len(m.beads)+1), Title: title, Status: "open", Type: "task"}
				m.beads = append(m.beads, b)
				m.input.SetValue("")
				return m, func() tea.Msg { return BeadAddedMsg{Bead: b} }
			}
			return m, func() tea.Msg {
				b, err := bdCreate(title)
				if err != nil {
					return bdErrorMsg{err: err.Error()}
				}
				return beadCreatedMsg{bead: *b}
			}
		}
		return m, nil
	}

	// Scroll the existing beads list
	switch msg.String() {
	case "up":
		if m.createScroll > 0 {
			m.createScroll--
		}
		return m, nil
	case "down":
		m.createScroll++
		return m, nil
	}

	// Forward everything else to textinput
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateList(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if len(m.beads) == 0 || m.loading {
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.MoveUp):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, m.keys.MoveDown):
		if m.cursor < len(m.beads)-1 {
			m.cursor++
		}
	case key.Matches(msg, m.keys.NextStatus):
		if m.demoMode {
			m.beads[m.cursor].Status = nextStatus(m.beads[m.cursor].Status)
			bead := m.beads[m.cursor]
			return m, func() tea.Msg { return BeadStatusChangedMsg{Bead: bead} }
		}
		bead := m.beads[m.cursor]
		next := nextStatus(bead.Status)
		id := bead.ID
		return m, func() tea.Msg {
			if err := bdSetStatus(id, next); err != nil {
				return bdErrorMsg{err: err.Error()}
			}
			return beadStatusUpdatedMsg{id: id, status: next}
		}
	case key.Matches(msg, m.keys.Delete):
		if m.demoMode {
			id := m.beads[m.cursor].ID
			m.beads = append(m.beads[:m.cursor], m.beads[m.cursor+1:]...)
			if m.cursor >= len(m.beads) && m.cursor > 0 {
				m.cursor--
			}
			return m, func() tea.Msg { return BeadDeletedMsg{ID: id} }
		}
		id := m.beads[m.cursor].ID
		return m, func() tea.Msg {
			if err := bdDelete(id); err != nil {
				return bdErrorMsg{err: err.Error()}
			}
			return beadDeletedInternalMsg{id: id}
		}
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// View / Overlay
// ---------------------------------------------------------------------------

// Render returns just the modal panel string without any background.
// Use this with lipgloss.NewLayer/NewCompositor for transparent overlays.
// Returns empty string when inactive.
func (m Model) Render(width int) string {
	if m.mode == modeInactive {
		return ""
	}
	switch m.mode {
	case modeCreate:
		return m.viewCreate(width)
	case modeList:
		return m.viewList(width)
	}
	return ""
}

// Overlay renders the modal centered over content (replaces background).
// For transparent overlays, use Render() with lipgloss layers instead.
func (m Model) Overlay(content string, width, height int) string {
	if m.mode == modeInactive {
		return content
	}
	var modal string
	switch m.mode {
	case modeCreate:
		modal = m.viewCreate(width)
	case modeList:
		modal = m.viewList(width)
	}

	// Fill background to full screen size
	bg := lipgloss.Place(width, height, lipgloss.Top, lipgloss.Left, content)

	// Layer modal on top, centered
	panelW := lipgloss.Width(modal)
	panelH := lipgloss.Height(modal)
	px := (width - panelW) / 2
	py := (height - panelH) / 2
	if px < 0 {
		px = 0
	}
	if py < 0 {
		py = 0
	}
	root := lipgloss.NewLayer(bg)
	overlay := lipgloss.NewLayer(modal).X(px).Y(py).Z(1)
	comp := lipgloss.NewCompositor(root, overlay)
	return comp.Render()
}

func (m Model) capWidth(preferred, maxWidth int) int {
	cap := maxWidth - 6
	if m.modalWidth > 0 && m.modalWidth < cap {
		cap = m.modalWidth
	}
	w := preferred
	if w > cap {
		w = cap
	}
	if w < 20 {
		w = 20
	}
	return w
}

func (m Model) viewCreate(maxWidth int) string {
	w := m.capWidth(55, maxWidth)
	var b strings.Builder

	b.WriteString(m.styles.Title.Render(m.createIcon + " New Bead"))
	b.WriteString("\n")

	if !m.ready {
		errText := m.errMsg
		if errText == "" {
			errText = "Checking beads..."
		}
		b.WriteString(m.styles.Error.Render(errText))
		b.WriteString("\n")
		b.WriteString(m.styles.Help.Render("esc: close"))
		return m.styles.Modal.Width(w).Render(b.String())
	}

	if m.errMsg != "" {
		b.WriteString(m.styles.Error.Render(m.errMsg))
		b.WriteString("\n")
	}

	b.WriteString(m.input.View())
	b.WriteString("\n")

	// Existing beads below input
	if m.loading {
		b.WriteString(m.styles.Dimmed.Render("  Loading..."))
	} else if len(m.beads) > 0 {
		var nOpen, nWIP, nClosed int
		for _, bead := range m.beads {
			switch bead.Status {
			case "open":
				nOpen++
			case "in_progress":
				nWIP++
			case "closed":
				nClosed++
			}
		}
		b.WriteString("\n")
		b.WriteString(m.styles.Dimmed.Render(fmt.Sprintf("  %d beads (%d open, %d wip, %d done)", len(m.beads), nOpen, nWIP, nClosed)))
		b.WriteString("\n")

		maxVisible := 8
		if m.height > 0 {
			v := (m.height - 16)
			if v < 4 {
				v = 4
			}
			if v > 12 {
				v = 12
			}
			maxVisible = v
		}

		start := m.createScroll
		if start > len(m.beads)-maxVisible {
			start = max(0, len(m.beads)-maxVisible)
		}
		end := start + maxVisible
		if end > len(m.beads) {
			end = len(m.beads)
		}

		if start > 0 {
			b.WriteString(m.styles.Dimmed.Render("  ↑ more"))
			b.WriteString("\n")
		}
		for i := start; i < end; i++ {
			bead := m.beads[i]
			ss := m.statusStyle(bead.Status)
			b.WriteString(fmt.Sprintf("  %s %s\n", ss.Render(statusIcon(bead.Status)), bead.Title))
		}
		if end < len(m.beads) {
			b.WriteString(m.styles.Dimmed.Render("  ↓ more"))
		}
	}

	b.WriteString("\n")
	b.WriteString(m.styles.Help.Render("enter: add  •  ↑↓: scroll  •  esc: close"))
	return m.styles.Modal.Width(w).Render(b.String())
}

func (m Model) viewList(maxWidth int) string {
	w := m.capWidth(60, maxWidth)
	var b strings.Builder

	b.WriteString(m.styles.Title.Render(m.listIcon + " Beads"))
	b.WriteString("\n")

	if !m.ready {
		errText := m.errMsg
		if errText == "" {
			errText = "Checking beads..."
		}
		b.WriteString(m.styles.Error.Render(errText))
		b.WriteString("\n")
		b.WriteString(m.styles.Help.Render("esc: close"))
		return m.styles.Modal.Width(w).Render(b.String())
	}

	if m.errMsg != "" {
		b.WriteString(m.styles.Error.Render(m.errMsg))
		b.WriteString("\n")
	}

	if m.loading {
		b.WriteString(m.styles.Dimmed.Render("  Loading..."))
	} else if len(m.beads) == 0 {
		b.WriteString(m.styles.Dimmed.Render("  No beads yet. Press " + m.keys.OpenCreate.Help().Key + " to create one."))
	} else {
		// Stats line
		var nOpen, nWIP, nClosed int
		for _, bead := range m.beads {
			switch bead.Status {
			case "open":
				nOpen++
			case "in_progress":
				nWIP++
			case "closed":
				nClosed++
			}
		}
		b.WriteString(m.styles.Dimmed.Render(fmt.Sprintf("  Total: %d issues (%d open, %d in progress, %d closed)", len(m.beads), nOpen, nWIP, nClosed)))
		b.WriteString("\n\n")

		for i, bead := range m.beads {
			ss := m.statusStyle(bead.Status)
			icon := ss.Render(statusIcon(bead.Status))
			status := ss.Render(fmt.Sprintf("%-11s", statusDisplay(bead.Status)))
			id := m.styles.Dimmed.Render(bead.ID)
			line := fmt.Sprintf(" %s %s %s %s", icon, status, bead.Title, id)

			if i == m.cursor {
				line = m.styles.SelectedRow.Render(">" + line)
			} else {
				line = " " + line
			}
			b.WriteString(line)
			if i < len(m.beads)-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	helpParts := []string{
		m.keys.MoveUp.Help().Key + "/" + m.keys.MoveDown.Help().Key + ": navigate",
		m.keys.NextStatus.Help().Key + ": cycle status",
		m.keys.Delete.Help().Key + ": delete",
		m.keys.Cancel.Help().Key + ": close",
	}
	b.WriteString(m.styles.Help.Render(strings.Join(helpParts, "  •  ")))
	return m.styles.Modal.Width(w).Render(b.String())
}

func (m Model) statusStyle(s string) lipgloss.Style {
	switch s {
	case "in_progress":
		return m.styles.StatusWIP
	case "closed":
		return m.styles.StatusDone
	default:
		return m.styles.StatusOpen
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
