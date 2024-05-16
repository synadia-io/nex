//go:build linux || windows

package node

import (
	"github.com/synadia-io/nex/cli/node/preflight"
	"github.com/synadia-io/nex/cli/node/up"
)

type NodeExtendedCmds struct {
	Preflight preflight.PreflightCmd `cmd:"" json:"-"`
	Up        up.UpCmd               `cmd:"" json:"-"`
}
