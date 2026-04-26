// Command bbt-demo is a minimal app for VHS recording with --demo flag.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/NSXBet/bbt"
)

type model struct {
	beads  bbt.Model
	width  int
	height int
}

func (m model) Init() tea.Cmd {
	return nil // skip bd check for demo
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		if !m.beads.Active() {
			switch msg.String() {
			case "q", "esc":
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.beads, cmd = m.beads.Update(msg)
	return m, cmd
}

func (m model) View() tea.View {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render("🔮 bbt demo")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Render("  a: new bead   l: list   q: quit")
	content := lipgloss.NewStyle().Padding(1, 2).Render(title + "\n\n" + help + "\n")
	content = m.beads.Overlay(content, m.width, m.height)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func main() {
	km := bbt.DefaultKeyMap()
	km.OpenCreate = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "new bead"))
	km.OpenList = key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "list beads"))

	bp := bbt.New(bbt.WithKeyMap(km), bbt.WithDemoMode())

	m := model{beads: bp}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
