//go:build !windows

package native

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/carlmjohnson/be"

	"github.com/synadia-io/nex/models"
)

// TestStopKillsForkedGrandchild pins the process-group kill (leg B / M6): a
// workload that forks a child which ignores SIGINT must have that child killed
// when the workload is stopped, not left orphaned. Before the fix the stop
// signalled only the direct child (the outer shell), so a signal-ignoring
// grandchild survived the stop and kept running on the host.
func TestStopKillsForkedGrandchild(t *testing.T) {
	s := newLifecycleState(t)

	shPath, err := exec.LookPath("sh")
	be.NilErr(t, err)

	pidFile := filepath.Join(t.TempDir(), "child.pid")

	// The workload (outer sh) forks an inner sh that ignores INT/TERM and
	// loops re-spawning sleep -- so it survives the graceful SIGINT and is
	// still alive at kill time -- records that inner sh's pid, then waits. The
	// inner sh is the grandchild a single-pid kill would leave orphaned.
	script := fmt.Sprintf(`sh -c 'trap "" INT TERM; while :; do sleep 3000; done' & echo $! > %s; wait`, pidFile)
	runReq, err := json.Marshal(StartRequest{Uri: "file://" + shPath, Argv: []string{"-c", script}})
	be.NilErr(t, err)

	req := &models.AgentStartWorkloadRequest{
		Request: models.StartWorkloadRequest{
			Name:              "forker",
			Namespace:         lifecycleNamespace,
			RunRequest:        string(runReq),
			WorkloadLifecycle: models.WorkloadLifecycleService,
			WorkloadType:      NEXLET_REGISTER_TYPE,
		},
	}

	be.NilErr(t, s.AddWorkload(lifecycleNamespace, "wl-fork", req))

	// Read back the inner sh's pid once the workload has written it.
	var childPid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, rerr := os.ReadFile(pidFile); rerr == nil {
			if p, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil && p > 0 {
				childPid = p
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	be.Nonzero(t, childPid)
	// Ensure the grandchild is cleaned up even if the assertion below fails.
	t.Cleanup(func() { _ = syscall.Kill(childPid, syscall.SIGKILL) })
	be.False(t, processGone(childPid)) // alive before the stop

	be.NilErr(t, s.RemoveWorkload(lifecycleNamespace, "wl-fork"))

	// Decisive assertion: the forked, signal-ignoring grandchild is gone, not
	// orphaned. Before the fix it survives the stop.
	gone := false
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if processGone(childPid) {
			gone = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	be.True(t, gone)
}
