package node

type ListCmd struct{}

func (l ListCmd) Run() error {
	return nil
}

func (l ListCmd) Validate() error {
	return nil
}
