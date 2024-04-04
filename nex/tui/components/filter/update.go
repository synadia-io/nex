package filter

import (
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/synadia-io/nex/nex/tui/messages"
)

func (m filter) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	m.logger.Debug("filter", slog.Any("msg", message))
	var cmd tea.Cmd
	switch msg := message.(type) {
	case messages.UpdateSizeMessage:
		m.width = msg.Width
		m.height = msg.Height
	}
	m.textinput, cmd = m.textinput.Update(message)
	return m, cmd
}
