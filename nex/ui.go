//go:build withui

package main

import (
	"github.com/choria-io/fisk"
	nexui "github.com/synadia-io/nex/ui"
)

func init() {
	ui := ncli.Command("ui", "Starts a web server for interacting with Nex")
	ui.Flag("port", "Port on which to run the UI").Default("8080").IntVar(&GuiOpts.Port)
	ui.Action(RunUI)
}

func RunUI(ctx *fisk.ParseContext) error {
	nexui.ServeUI(GuiOpts.Port)

	return nil
}
