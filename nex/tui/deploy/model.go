package deploy

import (
	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
)

type DeployModel struct {
	help help.Model
}

func NewDeployModel() DeployModel {
	return DeployModel{
		help: help.New(),
	}
}

func (m DeployModel) Init() tea.Cmd {
	return nil
}
func (m DeployModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}
func (m DeployModel) View() string {
	return "INSIDE DEPLOY MODEL"
}
