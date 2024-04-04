package filter

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Forward key.Binding
	Back    key.Binding
	Filter  key.Binding
}

func getKeyMap() keyMap {
	return keyMap{
		Forward: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "enter"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
	}
}
