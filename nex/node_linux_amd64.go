package main

import (
	"context"

	"github.com/choria-io/fisk"
	"github.com/sirupsen/logrus"
	nexnode "github.com/synadia-io/nex/internal/node"
)

func init() {
	node_up := nodes.Command("up", "Starts a NEX node")
	node_up.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)
	node_preflight := nodes.Command("preflight", "Checks system for node requirements and installs missing")
	node_preflight.Flag("force", "installs missing dependencies without prompt").Default("false").BoolVar(&NodeOpts.ForceDepInstall)
	node_preflight.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)
	node_up.Action(RunNodeUp)
	node_preflight.Action(RunNodePreflight)

}

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
