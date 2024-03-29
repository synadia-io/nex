package confirm

import (
	tea "github.com/charmbracelet/bubbletea"
)

type ConfirmModel struct {
	Message string
}

func NewConfirmModel(inMsg string) ConfirmModel {
	return ConfirmModel{
		Message: inMsg,
	}
}

func (m ConfirmModel) Init() tea.Cmd {
	return nil
}

func (m ConfirmModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	return nil, nil
}

func (m ConfirmModel) View() string {
	return ""
}
