package tui

// A simple program that opens the alternate screen buffer then counts down
// from 5 and then exits.

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/synadia-io/nex/nex/tui/home"
)

func StartTUI() error {
	init, err := InitTUI()
	if err != nil {
		return err
	}
	p := tea.NewProgram(init, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

func InitTUI() (tea.Model, error) {
	return model{
		state:    homeView,
		homeView: home.NewHomeModel(),
	}, nil
}
