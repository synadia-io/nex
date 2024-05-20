package main

type nodeOptions struct {
	List nodeListCmd `cmd:"" json:"-"`
	Info nodeInfoCmd `cmd:"" json:"-"`

	nodeExtendedCmds `json:"-"`
}
