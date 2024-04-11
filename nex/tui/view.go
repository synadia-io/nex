package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	borderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("838"))
)

func (m model) View() string {
	nctx := "NATS Context: " + m.nContext
	switch m.state {
	case natsContextView:
		return borderStyle.Height(m.height - 2).Width(m.width - 2).Render(m.natsContextView.View())
	case confirmView:
		return borderStyle.Height(m.height - 2).Width(m.width - 2).Render(m.confirmView.View())
	case deployView:
		return borderStyle.Height(m.height - 2).Width(m.width - 2).Render(m.deployView.View())
	default:
		return borderStyle.Height(m.height - 2).Width(m.width - 2).Render(lipgloss.JoinVertical(lipgloss.Top, nctx, m.homeView.View()))
	}
}
