package runner

import "github.com/charmbracelet/lipgloss"

var (
	catalogTitleStyle   = lipgloss.NewStyle().Bold(true)
	catalogHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
	catalogActiveStyle  = lipgloss.NewStyle().Bold(true)
	catalogDetailStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	catalogWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	catalogFooterStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)
