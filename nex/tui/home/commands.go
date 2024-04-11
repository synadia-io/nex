package home

import tea "github.com/charmbracelet/bubbletea"

func EnterNatsContext() tea.Cmd {
	return tea.SetWindowTitle("Inside NATS Context")
}

func EnterConfirm() tea.Cmd {
	return tea.SetWindowTitle("Inside Confirm")
}

func EnterDeploy() tea.Cmd {
	return tea.SetWindowTitle("Inside Deploy")
}
