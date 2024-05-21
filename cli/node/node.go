package node

type NodeOptions struct {
	List ListCmd `cmd:"" aliases:"ls" json:"-"`
	Info InfoCmd `cmd:"" json:"-"`

	NodeExtendedCmds `json:"-"`

	ServerPublicKey string `kong:"-" json:"-"`
}
