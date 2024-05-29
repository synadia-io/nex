//go:build linux || windows

package nexnode

type NodeExtendedCmds struct {
	Preflight PreflightCmd `cmd:"" json:"-"`
	Up        UpCmd        `cmd:"" json:"-"`
}
