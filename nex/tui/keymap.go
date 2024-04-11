package tui

import "github.com/charmbracelet/bubbles/key"

type keymap struct {
	Back key.Binding
	Quit key.Binding
}

// Keymap reusable key mappings shared across models
var Keymap = keymap{
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c", "q"),
		key.WithHelp("ctrl+c/q", "quit"),
	),
}
