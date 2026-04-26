# bbt — Beads Bubble Tea

A pluggable [Bubble Tea v2](https://charm.land/bubbletea) component that adds a modal todo/bead manager to any TUI app. Persists via the [`bd`](https://github.com/steveyegge/beads) CLI.

## Features

- **F7** opens a create modal with input + scrollable list of existing beads
- **F8** opens a list modal with status cycling, deletion, and navigation
- All data persisted through `bd` CLI — shows up in `bd list`
- Fully configurable keybinds and colors
- Drop-in: 3 lines to integrate into any Bubble Tea app

## Install

```bash
go get github.com/NSXBet/bbt
```

Requires `bd` in PATH. Install from [github.com/steveyegge/beads](https://github.com/steveyegge/beads).

## Quick Start

```go
import "github.com/NSXBet/bbt"

type myApp struct {
    beads bbt.Model
    // ...your fields
}

func (m myApp) Init() tea.Cmd {
    return tea.Batch(m.beads.Init(), /* your init */)
}

func (m myApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    m.beads, cmd = m.beads.Update(msg)
    if m.beads.Active() {
        return m, cmd // modal open, swallow input
    }
    // ...your update logic
    return m, cmd
}

func (m myApp) View() tea.View {
    content := m.renderYourApp()
    content = m.beads.Overlay(content, m.width, m.height)
    return tea.NewView(content)
}
```

## Configuration

```go
// All defaults
bp := bbt.New()

// Custom keybinds
km := bbt.DefaultKeyMap()
km.OpenCreate = key.NewBinding(key.WithKeys("f5"), key.WithHelp("f5", "new"))
bp := bbt.New(bbt.WithKeyMap(km))

// Custom colors
bp := bbt.New(bbt.WithColors(bbt.Colors{
    Border:     "#FF6600",
    StatusDone: "#00FF88",
}))

// Full config
bp := bbt.New(bbt.WithConfig(bbt.Config{
    KeyMap:      &km,
    Colors:      bbt.Colors{Border: "#7D56F4"},
    Placeholder: "What needs to be done?",
    CreateIcon:  "✦",
    ListIcon:    "✦",
}))
```

## Default Keybinds

| Key | Action |
|-----|--------|
| `f7` | Open create modal |
| `f8` | Open list modal |
| `enter` | Confirm / create bead |
| `esc` | Close modal |
| `↑/↓` or `k/j` | Navigate list |
| `tab` | Cycle status (open → in_progress → closed) |
| `d` | Delete selected bead |

## Messages

React to bead events in your app's Update:

```go
case bbt.BeadAddedMsg:
    // new bead created
case bbt.BeadStatusChangedMsg:
    // status cycled
case bbt.BeadDeletedMsg:
    // bead deleted
```

## Run the Example

```bash
bd init  # if no .beads/ in current dir
just run
```

## License

MIT
