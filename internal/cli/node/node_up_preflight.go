//go:build linux || windows

package node

type NodeExtendedCmds struct {
	Preflight PreflightCmd `cmd:"" json:"-"`
	Up        UpCmd        `cmd:"" json:"-"`
}
