package actors

import (
	"ergo.services/ergo/gen"
	"github.com/synadia-io/nex/node/options"
)

// NOTE: intentionally forcing a copy here so these options aren't mutable
// by the node
func CreateNodeApp(opts options.NodeOptions) gen.ApplicationBehavior {
	return &NodeApp{opts}
}

type NodeApp struct {
	opts options.NodeOptions
}

// Load invoked on loading application using method ApplicationLoad of gen.Node interface.
func (app *NodeApp) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "nex",
		Description: "Nex Node - A NATS Execution Engine",
		Mode:        gen.ApplicationModeTransient,
		Group: []gen.ApplicationMemberSpec{
			{
				Name:    "nexsup",
				Factory: createNexSupervisor,
				Args:    []any{app.opts},
			},
		},
	}, nil
}

// Start invoked once the application started
func (app *NodeApp) Start(mode gen.ApplicationMode) {}

// Terminate invoked once the application stopped
func (app *NodeApp) Terminate(reason error) {}
