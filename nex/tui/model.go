package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/nats-io/nats.go"
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

	nContext string
	nc       *nats.Conn

	state viewState

	homeView        tea.Model
	natsContextView tea.Model
	confirmView     tea.Model
	deployView      tea.Model
}

func (m model) isInitialized() bool {
	return m.height != 0 && m.width != 0
}
