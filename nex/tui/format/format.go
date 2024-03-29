package format

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	DocStyle   = lipgloss.NewStyle().Margin(0, 2)
	HelpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render
	ErrStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#bd534b")).Render
	AlertStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render
)
