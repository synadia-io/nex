//go:build linux || windows

package main

type nodeExtendedCmds struct {
	Preflight nodePreflightCmd `cmd:"" json:"-"`
	Up        nodeUpCmd        `cmd:"" json:"-"`
}
