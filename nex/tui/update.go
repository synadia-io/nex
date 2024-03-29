package tui

import (
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/synadia-io/nex/nex/tui/deploy"
	"github.com/synadia-io/nex/nex/tui/home"
	"github.com/synadia-io/nex/nex/tui/natsContext"
	"golang.org/x/term"
)

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if !m.isInitialized() {
		if _, ok := message.(tea.WindowSizeMsg); !ok {
			return m, nil
		}
	}

	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, _ = term.GetSize(int(os.Stdout.Fd()))
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "back":
			m.state = homeView
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	switch m.state {
	case homeView:
		m.homeView, cmd = m.homeView.Update(message)
		switch msg := message.(type) {
		case tea.KeyMsg:
			switch {
			case key.Matches(msg, home.Keys.EnterNatsContext):
				m.natsContextView = natsContext.NewNatsContextModel()
				m.state = natsContextView
			case key.Matches(msg, home.Keys.EnterDeployView):
				m.deployView = deploy.NewDeployModel()
				m.state = deployView
			}
		}
	case natsContextView:
		m.natsContextView, cmd = m.natsContextView.Update(message)
	case confirmView:
		m.confirmView, cmd = m.confirmView.Update(message)
	case deployView:
		m.deployView, cmd = m.deployView.Update(message)
	}

	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}
