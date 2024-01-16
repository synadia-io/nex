package main

import (
	"context"

	nexnode "github.com/ConnectEverything/nex/internal/node"
	"github.com/choria-io/fisk"
	"github.com/sirupsen/logrus"
)

func RunNodeUp(ctx *fisk.ParseContext) error {
	log := logrus.New()
	ctxx, cancel := context.WithCancel(context.Background())
	nexnode.CmdUp(Opts, NodeOpts, ctxx, cancel, log)
	<-ctxx.Done()
	return nil
}

func RunNodePreflight(ctx *fisk.ParseContext) error {
	log := logrus.New()
	ctxx, cancel := context.WithCancel(context.Background())
	nexnode.CmdPreflight(Opts, NodeOpts, ctxx, cancel, log)

	return nil
}
