package native

import (
	"context"
	"os"
	"time"

	"github.com/synadia-labs/nex/models"
)

type NativeProcesses map[string]*NativeProcess

type NativeProcess struct {
	cancel context.CancelFunc

	Process      *os.Process
	Name         string
	StartRequest *models.StartWorkloadRequest
	StartedAt    time.Time
	State        models.WorkloadState
	Restarts     int
	MaxRestarts  int
}
