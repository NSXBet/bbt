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
	beads  beadsplugin.Model
	width  int
	height int
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.beads.Init())
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
	case beadsplugin.BeadAddedMsg:
		// react
	case beadsplugin.BeadStatusChangedMsg:
		// react
	case beadsplugin.BeadDeletedMsg:
		// react
	}

	var cmd tea.Cmd
	m.beads, cmd = m.beads.Update(msg)
	if m.beads.Active() {
		return m, cmd
	}

	return m, cmd
}

func (m model) View() tea.View {
	title := lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Render("🔮 Beads Demo")

	var nOpen, nWIP, nDone int
	for _, b := range m.beads.Beads() {
		switch b.Status {
		case "open":
			nOpen++
		case "in_progress":
			nWIP++
		case "closed":
			nDone++
		}
	}

	stats := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(
		fmt.Sprintf("  total: %d  ○ open: %d  ◑ wip: %d  ● done: %d",
			len(m.beads.Beads()), nOpen, nWIP, nDone),
	)

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Render(
		"  f7      Create a bead\n" +
			"  f8      List beads\n" +
			"  q       Quit",
	)

	content := lipgloss.NewStyle().Padding(1, 2).Render(
		title + "\n\n" + stats + "\n\n" + help + "\n",
	)

	content = m.beads.Overlay(content, m.width, m.height)

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func main() {
	// All defaults:
	// bp := beadsplugin.New()

	// Custom config example:
	km := beadsplugin.DefaultKeyMap()
	km.OpenCreate = key.NewBinding(key.WithKeys("f7"), key.WithHelp("f7", "new bead"))
	km.OpenList = key.NewBinding(key.WithKeys("f8"), key.WithHelp("f8", "list beads"))

	bp := beadsplugin.New(beadsplugin.WithConfig(beadsplugin.Config{
		KeyMap:      &km,
		Colors:      beadsplugin.Colors{Border: "#7D56F4", Title: "#FF75B5"},
		Placeholder: "What needs to be done?",
		CreateIcon:  "✦",
		ListIcon:    "✦",
	}))

	m := model{beads: bp}
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
