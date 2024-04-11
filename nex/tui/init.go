package tui

import tea "github.com/charmbracelet/bubbletea"

func (m model) Init() tea.Cmd {
	return tea.SetWindowTitle("NEX TUI")
}
