package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type viewState int

const (
	homeView viewState = iota
	natsContextView
	confirmView
	deployView
)

type model struct {
	width  int
	height int

	state viewState

	homeView        tea.Model
	natsContextView tea.Model
	confirmView     tea.Model
	deployView      tea.Model
}

func (m model) isInitialized() bool {
	return m.height != 0 && m.width != 0
}
