package tui

import "github.com/charmbracelet/lipgloss"

var (
	pane      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	statusBar = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1)
)
