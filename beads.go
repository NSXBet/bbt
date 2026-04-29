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
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
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

func bdCreate(title, description, priority string) (*Bead, error) {
	args := []string{"create", title, "-t", "task", "--json"}
	if description != "" {
		args = append(args, "-d", description)
	}
	if priority != "" {
		args = append(args, "-p", priority)
	}
	out, err := bdRun(args...)
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
	KeyMap                 *KeyMap // Nil uses [DefaultKeyMap].
	Colors                 Colors  // Zero fields fall back to [DefaultColors].
	Placeholder            string  // Title input placeholder (default: "What needs to be done?").
	DescriptionPlaceholder string  // Description textarea placeholder.
	PriorityPlaceholder    string  // Priority input placeholder.
	DescriptionHeight      int     // Description textarea height in rows (default: 6).
	ModalWidth             int     // Maximum modal width; 0 means auto.
	CreateIcon             string  // Glyph before create modal title.
	ListIcon               string  // Glyph before list modal title.
}

// DefaultConfig returns the built-in configuration.
func DefaultConfig() Config {
	return Config{
		Colors:                 DefaultColors(),
		Placeholder:            "What needs to be done?",
		DescriptionPlaceholder: "Add more context (optional)…",
		PriorityPlaceholder:    "0-4 (default 2)",
		DescriptionHeight:      6,
		CreateIcon:             "✦",
		ListIcon:               "✦",
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
	Submit     key.Binding // Submit the create form.
	NextField  key.Binding // Move focus to next field in create form.
	PrevField  key.Binding // Move focus to previous field in create form.
	Cancel     key.Binding // Close modal.
	NextStatus key.Binding // Cycle status in list view.
	Delete     key.Binding // Delete selected bead in list view.
	MoveUp     key.Binding // Move cursor up in list view.
	MoveDown   key.Binding // Move cursor down in list view.
}

// DefaultKeyMap returns the built-in keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		OpenCreate: key.NewBinding(key.WithKeys("ctrl+9"), key.WithHelp("ctrl+9", "new bead")),
		OpenList:   key.NewBinding(key.WithKeys("ctrl+0"), key.WithHelp("ctrl+0", "list beads")),
		Submit:     key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "submit")),
		NextField:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
		PrevField:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev field")),
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

// WithPlaceholder sets the title input placeholder text.
func WithPlaceholder(s string) Option { return func(m *Model) { m.titleInput.Placeholder = s } }

// WithDescriptionPlaceholder sets the description textarea placeholder text.
func WithDescriptionPlaceholder(s string) Option {
	return func(m *Model) { m.descInput.Placeholder = s }
}

// WithPriorityPlaceholder sets the priority input placeholder text.
func WithPriorityPlaceholder(s string) Option {
	return func(m *Model) { m.priorityInput.Placeholder = s }
}

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
			m.titleInput.Placeholder = cfg.Placeholder
		}
		if cfg.DescriptionPlaceholder != "" {
			m.descInput.Placeholder = cfg.DescriptionPlaceholder
		}
		if cfg.PriorityPlaceholder != "" {
			m.priorityInput.Placeholder = cfg.PriorityPlaceholder
		}
		if cfg.DescriptionHeight > 0 {
			m.descInput.SetHeight(cfg.DescriptionHeight)
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
	fieldLabel  lipgloss.Style
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
		fieldLabel: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Title)).
			Bold(true),
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
// Field focus indices for the create form.
const (
	focusTitle = iota
	focusDescription
	focusPriority
	focusFieldCount
)

type Model struct {
	beads         []Bead
	mode          viewMode
	titleInput    textinput.Model
	descInput     textarea.Model
	priorityInput textinput.Model
	createFocus   int
	cursor        int
	keys          KeyMap
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

	ta := textarea.New()
	ta.Placeholder = "Add more context (optional)…"
	ta.Prompt = "│ "
	ta.ShowLineNumbers = false
	ta.SetWidth(40)
	ta.SetHeight(6)
	ta.CharLimit = 4096

	pi := textinput.New()
	pi.Placeholder = "0-4 (default 2)"
	pi.Prompt = "> "
	pi.SetWidth(10)
	pi.SetVirtualCursor(true)
	pi.CharLimit = 4

	c := DefaultColors()
	m := Model{
		keys:          DefaultKeyMap(),
		titleInput:    ti,
		descInput:     ta,
		priorityInput: pi,
		colors:        c,
		isDark:        true,
		styles:        buildStyles(c, true),
		createIcon:    "✦",
		listIcon:      "✦",
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
		m.resetCreateFields()
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

	// Forward non-key messages to the focused field (cursor blink, etc.).
	if m.mode == modeCreate && m.ready {
		return m.forwardToFocused(msg)
	}

	return m, nil
}

func (m *Model) resetCreateFields() {
	m.titleInput.SetValue("")
	m.descInput.Reset()
	m.priorityInput.SetValue("")
	m.createFocus = focusTitle
}

func (m Model) forwardToFocused(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.createFocus {
	case focusTitle:
		m.titleInput, cmd = m.titleInput.Update(msg)
	case focusDescription:
		m.descInput, cmd = m.descInput.Update(msg)
	case focusPriority:
		m.priorityInput, cmd = m.priorityInput.Update(msg)
	}
	return m, cmd
}

func (m *Model) focusField(idx int) tea.Cmd {
	m.titleInput.Blur()
	m.descInput.Blur()
	m.priorityInput.Blur()
	m.createFocus = idx
	switch idx {
	case focusTitle:
		return m.titleInput.Focus()
	case focusDescription:
		return m.descInput.Focus()
	case focusPriority:
		return m.priorityInput.Focus()
	}
	return nil
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
			if m.ready {
				m.resetCreateFields()
				focusCmd := m.focusField(focusTitle)
				if m.demoMode {
					m.loading = false
					return m, focusCmd
				}
				return m, focusCmd
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
		m.titleInput.Blur()
		m.descInput.Blur()
		m.priorityInput.Blur()
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

	switch {
	case key.Matches(msg, m.keys.Submit):
		return m.submitCreate()

	case key.Matches(msg, m.keys.NextField):
		cmd := m.focusField((m.createFocus + 1) % focusFieldCount)
		return m, cmd

	case key.Matches(msg, m.keys.PrevField):
		cmd := m.focusField((m.createFocus - 1 + focusFieldCount) % focusFieldCount)
		return m, cmd
	}

	// Forward the key to whichever field has focus.
	return m.forwardToFocused(msg)
}

func (m Model) submitCreate() (Model, tea.Cmd) {
	title := strings.TrimSpace(m.titleInput.Value())
	if title == "" {
		m.errMsg = "Title is required"
		return m, nil
	}
	description := strings.TrimSpace(m.descInput.Value())
	priority := strings.TrimSpace(m.priorityInput.Value())

	if m.demoMode {
		b := Bead{
			ID:     fmt.Sprintf("demo-%d", len(m.beads)+1),
			Title:  title,
			Status: StatusOpen,
			Type:   "task",
		}
		if priority != "" {
			if n, err := strconv.Atoi(priority); err == nil {
				b.Priority = n
			}
		}
		m.beads = append(m.beads, b)
		m.resetCreateFields()
		return m, func() tea.Msg { return BeadAddedMsg{Bead: b} }
	}
	return m, func() tea.Msg {
		b, err := bdCreate(title, description, priority)
		if err != nil {
			return bdErrorMsg{err: err.Error()}
		}
		return beadCreatedMsg{bead: *b}
	}
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
	w := m.capWidth(60, maxWidth)
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

	// Title field.
	b.WriteString(m.styles.fieldLabel.Render("Title"))
	b.WriteString("\n")
	b.WriteString(m.titleInput.View())
	b.WriteString("\n\n")

	// Description field — generous multi-line space.
	b.WriteString(m.styles.fieldLabel.Render("Description"))
	b.WriteString("\n")
	b.WriteString(m.descInput.View())
	b.WriteString("\n\n")

	// Priority field.
	b.WriteString(m.styles.fieldLabel.Render("Priority"))
	b.WriteString("\n")
	b.WriteString(m.priorityInput.View())
	b.WriteString("\n")

	b.WriteString(m.styles.help.Render(
		m.keys.NextField.Help().Key + "/" + m.keys.PrevField.Help().Key + ": fields  •  " +
			m.keys.Submit.Help().Key + ": submit  •  " +
			m.keys.Cancel.Help().Key + ": close",
	))
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
