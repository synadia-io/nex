package nexnode

import (
	"context"
	"net"
	"os"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	controlapi "github.com/ConnectEverything/nex/control-api"
	"github.com/firecracker-microvm/firecracker-go-sdk"
	log "github.com/sirupsen/logrus"
)

// Represents an instance of a single firecracker VM containing the nex agent.
type runningFirecracker struct {
	vmmCtx                context.Context
	vmmCancel             context.CancelFunc
	vmmID                 string
	machine               *firecracker.Machine
	ip                    net.IP
	agentClient           agentapi.AgentClient
	workloadStarted       time.Time
	machineStarted        time.Time
	workloadSpecification controlapi.RunRequest
	namespace             string
}

func (vm *runningFirecracker) Subscribe() (<-chan *agentapi.LogEntry, <-chan *agentapi.AgentEvent, error) {
	logs, err := vm.agentClient.SubscribeToLogs()
	if err != nil {
		return nil, nil, err
	}
	evts, err := vm.agentClient.SubscribeToEvents()
	if err != nil {
		return nil, nil, err
	}
	return logs, evts, nil
}

func (vm *runningFirecracker) shutDown() {

	log.WithField("vmid", vm.vmmID).
		WithField("ip", vm.ip).
		Info("Machine stopping")

	vm.machine.StopVMM()
	err := os.Remove(vm.machine.Cfg.SocketPath)
	if err != nil {
		log.WithError(err).Warn("Failed to delete firecracker socket")
	}

	// NOTE: we're not deleting the firecracker machine logs ... they're in a tempfs so they'll eventually
	// go away but we might want them kept around for troubleshooting

	rootFs := getRootFsPath(vm.vmmID)
	err = os.Remove(rootFs)
	if err != nil {
		log.WithError(err).Warn("Failed to delete firecracker rootfs")
	}
}
