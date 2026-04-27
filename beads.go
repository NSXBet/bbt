// Package bbt provides a pluggable [Bubble Tea v2] modal component for managing
// beads (todo items) within any TUI application.
//
// Data is persisted through the bd CLI (github.com/steveyegge/beads). The
// component renders as a centered overlay modal with two views: a create form
// and a list view with status cycling and deletion.
//
// Add bbt to any Bubble Tea app in three steps:
//
//  1. Embed [Model] in your app's model and call [New] with options.
//  2. Forward messages via [Model.Update] and short-circuit when [Model.Active].
//  3. Render with [Model.Overlay] or [Model.Render] in your View function.
//
// Use [WithDemoMode] to skip persistence for UI testing.
//
// [Bubble Tea v2]: https://charm.land/bubbletea
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
// Bead
// ---------------------------------------------------------------------------

// Status constants matching bd's status values.
const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusClosed     = "closed"
)

// Bead represents a single work item as returned by the bd CLI.
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
	case StatusOpen:
		return "○"
	case StatusInProgress:
		return "◑"
	case StatusClosed:
		return "●"
	default:
		return "?"
	}
}

func statusDisplay(s string) string {
	switch s {
	case StatusInProgress:
		return "in-progress"
	default:
		return s
	}
}

func nextStatus(s string) string {
	switch s {
	case StatusOpen:
		return StatusInProgress
	case StatusInProgress:
		return StatusClosed
	case StatusClosed:
		return StatusOpen
	default:
		return StatusOpen
	}
}

// ---------------------------------------------------------------------------
// bd CLI
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
		return nil, fmt.Errorf("parsing bd list output: %w", err)
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
		return nil, fmt.Errorf("parsing bd create output: %w", err)
	}
	return &b, nil
}

func bdSetStatus(id, status string) error {
	switch status {
	case StatusInProgress:
		_, err := bdRun("update", id, "--claim", "--json")
		return err
	case StatusClosed:
		_, err := bdRun("close", id, "--json")
		return err
	case StatusOpen:
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
// Configuration
// ---------------------------------------------------------------------------

// Colors holds hex color strings for the modal UI.
// Any zero-value field falls back to [DefaultColors].
type Colors struct {
	Border     string // Modal border color.
	Title      string // Modal title text color.
	StatusOpen string // Open status indicator color.
	StatusWIP  string // In-progress status indicator color.
	StatusDone string // Closed/done status indicator color.
	DimDark    string // Muted text on dark backgrounds.
	DimLight   string // Muted text on light backgrounds.
	SelDark    string // Selected row background on dark terminals.
	SelLight   string // Selected row background on light terminals.
}

// DefaultColors returns the built-in color palette.
func DefaultColors() Colors {
	return Colors{
		Border:     "#7D56F4",
		Title:      "#FF75B5",
		StatusOpen: "#888888",
		StatusWIP:  "#FFAA00",
		StatusDone: "#00CC66",
		DimDark:    "#555555",
		DimLight:   "#AAAAAA",
		SelDark:    "#4A4A6C",
		SelLight:   "#E8E8FF",
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

// Config groups all configuration for the bbt component.
type Config struct {
	KeyMap      *KeyMap // Nil uses [DefaultKeyMap].
	Colors      Colors  // Zero fields fall back to [DefaultColors].
	Placeholder string  // Text input placeholder (default: "What needs to be done?").
	ModalWidth  int     // Maximum modal width; 0 means auto.
	CreateIcon  string  // Glyph before create modal title.
	ListIcon    string  // Glyph before list modal title.
}

// DefaultConfig returns the built-in configuration.
func DefaultConfig() Config {
	return Config{
		Colors:      DefaultColors(),
		Placeholder: "What needs to be done?",
		CreateIcon:  "✦",
		ListIcon:    "✦",
	}
}

// ---------------------------------------------------------------------------
// KeyMap
// ---------------------------------------------------------------------------

// KeyMap defines all keybindings for the bbt component.
// Use [DefaultKeyMap] as a starting point and override individual bindings.
type KeyMap struct {
	OpenCreate key.Binding // Open the create modal.
	OpenList   key.Binding // Open the list modal.
	Confirm    key.Binding // Confirm input / create bead.
	Cancel     key.Binding // Close modal.
	NextStatus key.Binding // Cycle status in list view.
	Delete     key.Binding // Delete selected bead in list view.
	MoveUp     key.Binding // Move cursor up in list view.
	MoveDown   key.Binding // Move cursor down in list view.
}

// DefaultKeyMap returns the built-in keybindings.
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
// Options
// ---------------------------------------------------------------------------

// Option configures [New].
type Option func(*Model)

// WithKeyMap overrides the full keymap.
func WithKeyMap(km KeyMap) Option { return func(m *Model) { m.keys = km } }

// WithPlaceholder sets the create-modal input placeholder text.
func WithPlaceholder(s string) Option { return func(m *Model) { m.input.Placeholder = s } }

// WithColors overrides the color palette. Zero-value fields keep defaults.
func WithColors(c Colors) Option {
	return func(m *Model) {
		m.colors = c.withDefaults()
		m.styles = buildStyles(m.colors, m.isDark)
	}
}

// WithDemoMode disables bd CLI calls entirely. Beads are stored in-memory
// only, making this useful for testing or recording demos.
func WithDemoMode() Option {
	return func(m *Model) {
		m.checked = true
		m.ready = true
		m.demoMode = true
	}
}

// WithConfig applies a full [Config]. Nil KeyMap fields keep defaults.
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
// Styles (internal)
// ---------------------------------------------------------------------------

type styles struct {
	modal       lipgloss.Style
	title       lipgloss.Style
	selectedRow lipgloss.Style
	statusOpen  lipgloss.Style
	statusWIP   lipgloss.Style
	statusDone  lipgloss.Style
	help        lipgloss.Style
	dimmed      lipgloss.Style
	errText     lipgloss.Style
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
		modal: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(c.Border)).
			Padding(1, 2),
		title: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Title)).
			Bold(true).
			MarginBottom(1),
		selectedRow: lipgloss.NewStyle().Background(sel),
		statusOpen:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.StatusOpen)),
		statusWIP:   lipgloss.NewStyle().Foreground(lipgloss.Color(c.StatusWIP)),
		statusDone:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.StatusDone)),
		help:        lipgloss.NewStyle().Foreground(dim).MarginTop(1),
		dimmed:      lipgloss.NewStyle().Foreground(dim),
		errText:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4444")).Bold(true),
	}
}

func (s styles) statusStyle(status string) lipgloss.Style {
	switch status {
	case StatusInProgress:
		return s.statusWIP
	case StatusClosed:
		return s.statusDone
	default:
		return s.statusOpen
	}
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// BeadAddedMsg is emitted when a new bead is created successfully.
type BeadAddedMsg struct{ Bead Bead }

// BeadStatusChangedMsg is emitted when a bead's status is cycled.
type BeadStatusChangedMsg struct{ Bead Bead }

// BeadDeletedMsg is emitted when a bead is deleted.
type BeadDeletedMsg struct{ ID string }

// Internal messages — not exported.
type (
	beadsLoadedMsg        struct{ beads []Bead }
	bdErrorMsg            struct{ err string }
	bdCheckDoneMsg        struct{ err error }
	beadCreatedMsg        struct{ bead Bead }
	beadStatusUpdatedMsg  struct{ id, status string }
	beadDeletedInternalMsg struct{ id string }
)

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type viewMode int

const (
	modeInactive viewMode = iota
	modeCreate
	modeList
)

// Model is the bbt Bubble Tea component. Embed it in your application model
// and forward messages via [Model.Update].
type Model struct {
	beads        []Bead
	mode         viewMode
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
	checked      bool // bd availability check completed
	ready        bool // bd is available
	loading      bool // async operation in flight
	demoMode     bool // skip bd, in-memory only
}

// New creates a bbt component with the given options.
// Call [Model.Init] from your application's Init to check bd availability.
func New(opts ...Option) Model {
	ti := textinput.New()
	ti.Placeholder = "What needs to be done?"
	ti.Prompt = "> "
	ti.SetWidth(40)
	ti.SetVirtualCursor(true)
	ti.CharLimit = 256

	c := DefaultColors()
	m := Model{
		keys:       DefaultKeyMap(),
		input:      ti,
		colors:     c,
		isDark:     true,
		styles:     buildStyles(c, true),
		createIcon: "✦",
		listIcon:   "✦",
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// Init returns a command that verifies bd availability.
// Batch this with your application's own Init commands.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		return bdCheckDoneMsg{err: bdCheck()}
	}
}

// Active reports whether a modal is currently open.
// When true, the host application should not process key events.
func (m Model) Active() bool { return m.mode != modeInactive }

// Beads returns the current in-memory bead list.
func (m Model) Beads() []Bead { return m.beads }

// SetSize updates the available terminal dimensions for overlay positioning.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetDark switches between dark and light color schemes.
func (m *Model) SetDark(isDark bool) {
	m.isDark = isDark
	m.styles = buildStyles(m.colors, isDark)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update processes a Bubble Tea message. Call this from your app's Update
// and check [Model.Active] to decide whether to swallow further input.
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

	// Forward non-key messages to textinput (cursor blink, etc.).
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
					return m, m.input.Focus()
				}
				m.loading = true
				cmd := m.input.Focus()
				return m, tea.Batch(cmd, loadBeadsCmd())
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
				return m, loadBeadsCmd()
			}
			return m, nil
		}
		return m, nil
	}

	// Cancel closes any open modal.
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

func loadBeadsCmd() tea.Cmd {
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
		if title == "" {
			return m, nil
		}
		if m.demoMode {
			b := Bead{
				ID:     fmt.Sprintf("demo-%d", len(m.beads)+1),
				Title:  title,
				Status: StatusOpen,
				Type:   "task",
			}
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

	// Scroll existing beads list with arrow keys.
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

	// Forward remaining keys to textinput.
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
// View
// ---------------------------------------------------------------------------

// Render returns the modal panel string without any background.
// Use this with lipgloss.NewLayer/NewCompositor for transparent overlays.
// Returns an empty string when no modal is active.
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

// Overlay renders the modal centered over content using lipgloss layers.
// The background content remains visible around and behind the modal.
// Returns content unchanged when no modal is active.
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

	bg := lipgloss.Place(width, height, lipgloss.Top, lipgloss.Left, content)

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
	return lipgloss.NewCompositor(root, overlay).Render()
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

	b.WriteString(m.styles.title.Render(m.createIcon + " New Bead"))
	b.WriteString("\n")

	if !m.ready {
		errText := m.errMsg
		if errText == "" {
			errText = "Checking beads..."
		}
		b.WriteString(m.styles.errText.Render(errText))
		b.WriteString("\n")
		b.WriteString(m.styles.help.Render("esc: close"))
		return m.styles.modal.Width(w).Render(b.String())
	}

	if m.errMsg != "" {
		b.WriteString(m.styles.errText.Render(m.errMsg))
		b.WriteString("\n")
	}

	b.WriteString(m.input.View())
	b.WriteString("\n")

	// Existing beads below input.
	if m.loading {
		b.WriteString(m.styles.dimmed.Render("  Loading..."))
	} else if len(m.beads) > 0 {
		var nOpen, nWIP, nClosed int
		for _, bead := range m.beads {
			switch bead.Status {
			case StatusOpen:
				nOpen++
			case StatusInProgress:
				nWIP++
			case StatusClosed:
				nClosed++
			}
		}
		b.WriteString("\n")
		b.WriteString(m.styles.dimmed.Render(
			fmt.Sprintf("  %d beads (%d open, %d wip, %d done)", len(m.beads), nOpen, nWIP, nClosed),
		))
		b.WriteString("\n")

		maxVisible := clamp((m.height-16), 4, 12)
		start := m.createScroll
		if start > len(m.beads)-maxVisible {
			start = max(0, len(m.beads)-maxVisible)
		}
		end := start + maxVisible
		if end > len(m.beads) {
			end = len(m.beads)
		}

		if start > 0 {
			b.WriteString(m.styles.dimmed.Render("  ↑ more"))
			b.WriteString("\n")
		}
		for i := start; i < end; i++ {
			bead := m.beads[i]
			ss := m.styles.statusStyle(bead.Status)
			b.WriteString(fmt.Sprintf("  %s %s\n", ss.Render(statusIcon(bead.Status)), bead.Title))
		}
		if end < len(m.beads) {
			b.WriteString(m.styles.dimmed.Render("  ↓ more"))
		}
	}

	b.WriteString("\n")
	b.WriteString(m.styles.help.Render("enter: add  •  ↑↓: scroll  •  esc: close"))
	return m.styles.modal.Width(w).Render(b.String())
}

func (m Model) viewList(maxWidth int) string {
	w := m.capWidth(60, maxWidth)
	var b strings.Builder

	b.WriteString(m.styles.title.Render(m.listIcon + " Beads"))
	b.WriteString("\n")

	if !m.ready {
		errText := m.errMsg
		if errText == "" {
			errText = "Checking beads..."
		}
		b.WriteString(m.styles.errText.Render(errText))
		b.WriteString("\n")
		b.WriteString(m.styles.help.Render("esc: close"))
		return m.styles.modal.Width(w).Render(b.String())
	}

	if m.errMsg != "" {
		b.WriteString(m.styles.errText.Render(m.errMsg))
		b.WriteString("\n")
	}

	if m.loading {
		b.WriteString(m.styles.dimmed.Render("  Loading..."))
	} else if len(m.beads) == 0 {
		b.WriteString(m.styles.dimmed.Render(
			"  No beads yet. Press " + m.keys.OpenCreate.Help().Key + " to create one.",
		))
	} else {
		// Stats line.
		var nOpen, nWIP, nClosed int
		for _, bead := range m.beads {
			switch bead.Status {
			case StatusOpen:
				nOpen++
			case StatusInProgress:
				nWIP++
			case StatusClosed:
				nClosed++
			}
		}
		b.WriteString(m.styles.dimmed.Render(
			fmt.Sprintf("  Total: %d issues (%d open, %d in progress, %d closed)", len(m.beads), nOpen, nWIP, nClosed),
		))
		b.WriteString("\n\n")

		for i, bead := range m.beads {
			icon := statusIcon(bead.Status)
			status := fmt.Sprintf("%-11s", statusDisplay(bead.Status))

			if i == m.cursor {
				raw := fmt.Sprintf("> %s %s %s %s", icon, status, bead.Title, bead.ID)
				for len(raw) < w {
					raw += " "
				}
				b.WriteString(m.styles.selectedRow.Render(raw))
			} else {
				ss := m.styles.statusStyle(bead.Status)
				iconR := ss.Render(icon)
				statusR := ss.Render(status)
				idR := m.styles.dimmed.Render(bead.ID)
				b.WriteString(fmt.Sprintf("  %s %s %s %s", iconR, statusR, bead.Title, idR))
			}
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
	b.WriteString(m.styles.help.Render(strings.Join(helpParts, "  •  ")))
	return m.styles.modal.Width(w).Render(b.String())
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
