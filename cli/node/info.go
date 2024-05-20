package node

type InfoCmd struct {
	Id string `arg:"" required:""`
}

func (i InfoCmd) Run() error {
	return nil
}
func (i InfoCmd) Validate() error {
	return nil
}
