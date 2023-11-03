package nexnode

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/sirupsen/logrus"
)

const (
	VERSION = "0.0.1"
)

func NewMachineManager(ctx context.Context, cancel context.CancelFunc, nc *nats.Conn, config *NodeConfiguration, log *logrus.Logger) *MachineManager {
	// Create a new User KeyPair
	server, _ := nkeys.CreateServer()
	return &MachineManager{
		rootContext: ctx,
		rootCancel:  cancel,
		config:      config,
		nc:          nc,
		log:         log,
		kp:          server,
		allVms:      make(map[string]runningFirecracker),
		warmVms:     make(chan runningFirecracker, config.MachinePoolSize-1),
	}
}
