package main

type nodeInfoCmd struct {
	Id string `arg:"" required:""`
}

func (i nodeInfoCmd) Run() error {
	return nil
}

func (i nodeInfoCmd) Validate() error {
	return nil
}
