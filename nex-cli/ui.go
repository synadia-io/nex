package main

import (
	nexui "github.com/ConnectEverything/nex/nex-ui"
	"github.com/choria-io/fisk"
)

func RunUI(ctx *fisk.ParseContext) error {
	nexui.ServeUI(GuiOpts.Port)

	return nil
}
