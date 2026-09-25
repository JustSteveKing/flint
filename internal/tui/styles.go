package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	stepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	headingStyle = lipgloss.NewStyle().Bold(true)
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	warnStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	errStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	okStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	logStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	codeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("180"))
	cursorStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	boxStyle     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
)
