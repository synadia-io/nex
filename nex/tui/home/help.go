package home

import (
	"github.com/charmbracelet/bubbles/key"
)

type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	Select key.Binding
	Help   key.Binding
	Quit   key.Binding

	EnterNatsContext key.Binding
	EnterDeployView  key.Binding
}

// ShortHelp returns keybindings to be shown in the mini help view. It's part
// of the key.Map interface.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.EnterNatsContext, k.EnterDeployView, k.Quit}
}

// FullHelp returns keybindings for the expanded help view. It's part of the
// key.Map interface.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},                 // first column
		{k.EnterNatsContext, k.EnterDeployView, k.Quit}, // second column
	}
}

var Keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "move down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "move left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "move right"),
	),
	Select: key.NewBinding(
		key.WithKeys("enter", "E"),
		key.WithHelp("enter", "select"),
	),

	Help: key.NewBinding(
		key.WithKeys("?", "H"),
		key.WithHelp("?", "toggle help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "esc", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	EnterNatsContext: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "nats context editor"),
	),
	EnterDeployView: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "deploy workload"),
	),
}
