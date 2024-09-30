package actors

import (
	"ergo.services/ergo/gen"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/models"
)

const nexApplicationName = "nex"
const nexApplicationDescription = "Nex Node - A NATS Execution Engine"
const nexSupervisorName = "nexsup"

// NOTE: intentionally forcing a copy here so these options aren't mutable
// by the node
func CreateNodeApp(nc *nats.Conn, opts models.NodeOptions) gen.ApplicationBehavior {
	return &NodeApp{
		nc,
		opts,
	}
}

type NodeApp struct {
	nc   *nats.Conn
	opts models.NodeOptions
}

// Load invoked on loading application using method ApplicationLoad of gen.Node interface.
func (app *NodeApp) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	var nodeID string // FIXME!!! this should be part of the nexNode and NodeApp structs

	return gen.ApplicationSpec{
		Name:        nexApplicationName,
		Description: nexApplicationDescription,
		Mode:        gen.ApplicationModeTransient,
		Group: []gen.ApplicationMemberSpec{
			{
				Name:    nexSupervisorName,
				Factory: createNexSupervisor,
				Args:    []any{nodeID, app.nc, app.opts},
			},
		},
	}, nil
}

// Start invoked once the application started
func (app *NodeApp) Start(mode gen.ApplicationMode) {}

// Terminate invoked once the application stopped
func (app *NodeApp) Terminate(reason error) {}
