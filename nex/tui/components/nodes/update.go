package nodes

import (
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/synadia-io/nex/nex/tui/messages"
)

func (m nodes) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	m.logger.Debug("nodes", slog.Any("msg", message))
	switch msg := message.(type) {
	case messages.UpdateSizeMessage:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}
