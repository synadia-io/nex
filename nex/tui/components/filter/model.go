package filter

import (
	"log/slog"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
)

type filter struct {
	width, height int
	logger        *slog.Logger
	prefix        string
	keyMap        keyMap
	textinput     textinput.Model
	compact       bool
	suffix        string
}

func NewFilter(l *slog.Logger, prefix string) filter {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Cursor.SetMode(cursor.CursorHide)
	return filter{
		logger:    l,
		prefix:    prefix,
		keyMap:    getKeyMap(),
		textinput: ti,
	}
}
func (m *filter) SetSuffix(suffix string) {
	m.suffix = suffix
}
