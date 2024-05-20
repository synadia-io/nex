package node

type NodeOptions struct {
	List ListCmd `cmd:"" json:"-"`
	Info InfoCmd `cmd:"" json:"-"`

	NodeExtendedCmds `json:"-"`
}
