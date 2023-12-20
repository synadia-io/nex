package nexcli

import (
	"github.com/nats-io/natscli/columns"
)

func newColumns(heading string, a ...any) *columns.Writer {
	w := columns.New(heading, a...)
	w.SetColorScheme("cyan")
	w.SetHeading(heading, a...)

	return w
}
