package natsContext

import (
	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
)

type NatsContextModel struct {
	help help.Model
}

func NewNatsContextModel() NatsContextModel {
	return NatsContextModel{
		help: help.New(),
	}
}

func (m NatsContextModel) Init() tea.Cmd {
	return nil
}

func (m NatsContextModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m NatsContextModel) View() string {
	return "INSIDE NATS CONTEXT MODEL"
}
