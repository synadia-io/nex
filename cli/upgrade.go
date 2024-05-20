package main

type upgradeOptions struct{}

func (u upgradeOptions) Run(cfg globals) error {
	if cfg.Check {
		return u.Table()
	}
	return nil
}

func (u upgradeOptions) Validate() error {
	return nil
}
