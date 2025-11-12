package native

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/synadia-io/nex/models"
)

type NativeProcesses map[string]*NativeProcess

type NativeProcess struct {
	sync.RWMutex
	cancel context.CancelFunc

	Process      *os.Process
	Name         string
	StartRequest models.StartWorkloadRequest
	StartedAt    time.Time
	State        models.WorkloadState
	Restarts     int
	MaxRestarts  int
}

func (n *NativeProcess) SetState(inState models.WorkloadState) {
	n.Lock()
	defer n.Unlock()

	n.State = inState
}

func (n *NativeProcess) GetState() models.WorkloadState {
	n.RLock()
	defer n.RUnlock()

	return n.State
}
