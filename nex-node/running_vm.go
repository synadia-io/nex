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
	workloadSpecification *controlapi.RunRequest
}

func (vm *runningFirecracker) SubmitWorkload(file *os.File, request controlapi.RunRequest) error {
	_, err := vm.agentClient.PostWorkload(file.Name())
	if err != nil {
		return err
	}
	vm.workloadStarted = time.Now().UTC()
	vm.workloadSpecification = &request
	return nil
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

func (vm runningFirecracker) shutDown() {
	log.WithField("ip", vm.ip).Info("stopping")
	vm.machine.StopVMM()
	err := os.Remove(vm.machine.Cfg.SocketPath)
	if err != nil {
		log.WithError(err).Warn("Failed to delete firecracker socket")
	}
	err = os.Remove("/tmp/rootfs-" + vm.vmmID + ".ext4")
	if err != nil {
		log.WithError(err).Warn("Failed to delete firecracker rootfs")
	}
}
