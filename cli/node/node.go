package node

import (
	"github.com/synadia-io/nex/cli/node/info"
	"github.com/synadia-io/nex/cli/node/ls"
)

type NodeOptions struct {
	List ls.ListCmd   `cmd:"" json:"-"`
	Info info.InfoCmd `cmd:"" json:"-"`

	NodeExtendedCmds `json:"-"`
}
