package header

import "github.com/charmbracelet/lipgloss"
import "github.com/synadia-io/nex/nex/tui/components"

func (m Header) View() string {
	return components.HeaderBorder.
		Render(
			lipgloss.JoinVertical(lipgloss.Top,
				m.logo,
				lipgloss.JoinHorizontal(lipgloss.Left,
					m.version, " | ", m.natsContext,
				),
			),
		)
}
