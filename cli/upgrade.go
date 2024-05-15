package main

type upgradeOptions struct{}

func (u upgradeOptions) Run(ctx Context) error {
	return nil
}

func (u upgradeOptions) Table() error {
	return nil
}
