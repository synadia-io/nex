package header

import (
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
)

type Header struct {
	logger                                         *slog.Logger
	width, height                                  int
	logo, logoColor, natsContext, version, keyHelp string
	compact                                        bool
}

func NewHeader(l *slog.Logger, logo string, logoColor string, natsContext, version, keyHelp string) (m tea.Model) {
	return Header{
		logger:      l,
		logo:        logo,
		logoColor:   logoColor,
		natsContext: natsContext,
		version:     version,
		keyHelp:     keyHelp,
	}
}
