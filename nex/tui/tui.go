package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/synadia-io/nex/nex/tui/home"
)

func StartTUI(nContext string) error {
	init := model{
		state:    homeView,
		nContext: nContext,
		homeView: home.NewHomeModel(0, 0, nil),
	}

	p := tea.NewProgram(init, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}
