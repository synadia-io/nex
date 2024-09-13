package actors

import "ergo.services/ergo/gen"

func CreateNodeApp() gen.ApplicationBehavior {
	return &NodeApp{}
}

type NodeApp struct{}

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
			},
		},
	}, nil
}

// Start invoked once the application started
func (app *NodeApp) Start(mode gen.ApplicationMode) {}

// Terminate invoked once the application stopped
func (app *NodeApp) Terminate(reason error) {}
