package main

type lameDuckOptions struct{}

func (l lameDuckOptions) Run(ctx Context) error {
	return nil
}

func (l lameDuckOptions) Table() error {
	return nil
}
