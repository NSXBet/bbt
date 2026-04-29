# bbt — Beads Bubble Tea

A pluggable [Bubble Tea v2](https://charm.land/bubbletea) component that adds a modal todo/bead manager to any TUI app. Persists via the [`bd`](https://github.com/steveyegge/beads) CLI.

## Features

- **Ctrl+9** opens a create modal with Title, Description, and Priority fields
- **Ctrl+0** opens a list modal with status cycling, deletion, and navigation
- All data persisted through `bd` CLI — shows up in `bd list`
- Fully configurable keybinds and colors
- Transparent overlay — background content visible behind modal
- Demo mode for testing without `bd` installed
- Drop-in: 3 lines to integrate into any Bubble Tea app
- No CGO, no C dependencies — pure Go

## Install

```bash
go get github.com/NSXBet/bbt
```

### Prerequisites

- Go 1.26+
- [Bubble Tea v2](https://charm.land/bubbletea) (pulled automatically)
- [`bd`](https://github.com/steveyegge/beads) CLI in PATH (for persistence)

Install bd:
```bash
go install github.com/steveyegge/beads/cmd/bd@latest
```

## Quick Start

```go
import "github.com/NSXBet/bbt"

type myApp struct {
    todos  bbt.Model
    width  int
    height int
}

func (m myApp) Init() tea.Cmd {
    return tea.Batch(m.todos.Init(), /* your init */)
}

func (m myApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // Forward to bbt first
    var cmd tea.Cmd
    m.todos, cmd = m.todos.Update(msg)
    if m.todos.Active() {
        return m, cmd // modal open, swallow input
    }
    // ...your normal update logic
    return m, cmd
}

func (m myApp) View() tea.View {
    content := m.renderYourApp()
    // Option A: simple overlay (replaces background under modal)
    content = m.todos.Overlay(content, m.width, m.height)
    return tea.NewView(content)
}
```

### Transparent Overlay (lipgloss layers)

For apps that use `lipgloss.NewLayer`/`NewCompositor`, use `Render()` instead of `Overlay()`:

```go
func (m myApp) View() tea.View {
    bg := m.renderYourApp()
    v := tea.NewView("")
    v.AltScreen = true

    if m.todos.Active() {
        panel := m.todos.Render(m.width)
        panelW := lipgloss.Width(panel)
        panelH := lipgloss.Height(panel)
        px := (m.width - panelW) / 2
        py := (m.height - panelH) / 2

        root := lipgloss.NewLayer(bg)
        modal := lipgloss.NewLayer(panel).X(px).Y(py).Z(1)
        comp := lipgloss.NewCompositor(root, modal)
        v.SetContent(comp.Render())
    } else {
        v.SetContent(bg)
    }
    return v
}
```

## Configuration

```go
// All defaults
bp := bbt.New()

// Custom keybinds
km := bbt.DefaultKeyMap()
km.OpenCreate = key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "new"))
km.OpenList = key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "list"))
bp := bbt.New(bbt.WithKeyMap(km))

// Custom colors
bp := bbt.New(bbt.WithColors(bbt.Colors{
    Border:     "#FF6600",
    StatusDone: "#00FF88",
}))

// Full config
bp := bbt.New(bbt.WithConfig(bbt.Config{
    KeyMap:      &km,
    Colors:      bbt.Colors{Border: "#7D56F4", Title: "#FF75B5"},
    Placeholder: "What needs to be done?",
    CreateIcon:  "✦",
    ListIcon:    "✦",
}))

// Demo mode (no bd required, in-memory only)
bp := bbt.New(bbt.WithDemoMode())
```

## Default Keybinds

| Key | Action |
|-----|--------|
| `ctrl+9` | Open create modal |
| `ctrl+0` | Open list modal |
| `enter` | Confirm / create bead |
| `esc` | Close modal |
| `↑/↓` or `k/j` | Navigate list |
| `tab` | Cycle status (open → in_progress → closed) |
| `d` | Delete selected bead |
| `↑/↓` (in create) | Scroll existing beads list |

## Colors

All colors are configurable via `bbt.Colors{}`. Any zero-value field uses the default:

| Field | Default | Description |
|-------|---------|-------------|
| `Border` | `#7D56F4` | Modal border |
| `Title` | `#FF75B5` | Modal title |
| `StatusOpen` | `#888888` | Open status color |
| `StatusWIP` | `#FFAA00` | In-progress color |
| `StatusDone` | `#00CC66` | Done/closed color |
| `DimDark` | `#555555` | Muted text (dark bg) |
| `DimLight` | `#AAAAAA` | Muted text (light bg) |
| `SelDark` | `#4A4A6C` | Selected row bg (dark) |
| `SelLight` | `#E8E8FF` | Selected row bg (light) |

## Messages

React to bead events in your app's Update:

```go
case bbt.BeadAddedMsg:
    fmt.Println("Created:", msg.Bead.Title)
case bbt.BeadStatusChangedMsg:
    fmt.Println("Status changed:", msg.Bead.ID, msg.Bead.Status)
case bbt.BeadDeletedMsg:
    fmt.Println("Deleted:", msg.ID)
```

## API Reference

### Constructor

```go
func New(opts ...Option) Model
```

### Options

| Option | Description |
|--------|-------------|
| `WithKeyMap(km)` | Override all keybinds |
| `WithColors(c)` | Override color palette |
| `WithConfig(cfg)` | Full configuration |
| `WithPlaceholder(s)` | Input placeholder text |
| `WithDemoMode()` | Skip bd, in-memory only |

### Methods

| Method | Description |
|--------|-------------|
| `Init() tea.Cmd` | Check bd availability (batch with your Init) |
| `Update(msg) (Model, tea.Cmd)` | Handle messages |
| `Active() bool` | True when modal is open |
| `Beads() []Bead` | Current bead list |
| `Overlay(content, w, h) string` | Render modal over content (uses lipgloss layers) |
| `Render(w) string` | Return just the modal panel (for custom compositing) |
| `SetSize(w, h)` | Update terminal dimensions |
| `SetDark(bool)` | Switch light/dark styles |

## Run the Example

```bash
git clone git@github.com:NSXBet/bbt.git
cd bbt
bd init        # initialize beads database
just run       # run the example app
```

## How It Works

1. `Init()` runs `bd list --json --limit=1` to verify bd is available
2. Opening the create modal loads existing beads via `bd list --json`
3. Creating a bead runs `bd create "title" -t task --json`
4. Status cycling uses `bd update --claim`, `bd close`, or `bd reopen`
5. Deletion runs `bd delete <id> --force`

All operations are async (tea.Cmd) — the TUI never blocks.

## License

MIT
