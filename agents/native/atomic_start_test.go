//go:build !windows

package native

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/synadia-io/nex/models"
)

// The occupancy guard in NativeAgent.StartWorkload and the map insert in
// startWorkload used to run under separate lock acquisitions, so two starts
// that both passed the guard (the resume replay runs one goroutine per
// workload beside the endpoint handler) both inserted: the loser's generation
// was overwritten in the map and its process kept running untracked. The
// guard must live in the same critical section as the insert, which these
// tests exercise by driving the state layer directly -- the position a racing
// caller reaches after the outer check has already passed.

// A fresh start for an id that is already running must be refused by the
// state layer itself, and must leave the original generation tracked.
func TestAddWorkloadRefusesOccupiedId(t *testing.T) {
	s := newLifecycleState(t)
	const id = "wl-atomic-add"

	be.NilErr(t, s.AddWorkload(lifecycleNamespace, id, serviceRequest(t, "gen-a", "30")))
	pidA := runningPid(t, s, lifecycleNamespace, id)
	genA := s.getWorkload(lifecycleNamespace, id)

	err := s.AddWorkload(lifecycleNamespace, id, serviceRequest(t, "gen-b", "30"))
	if err == nil {
		// Pre-fix behavior: the insert overwrote gen-a. Register the
		// interloper for cleanup so a failing run does not leak its process.
		runningPid(t, s, lifecycleNamespace, id)
	}
	be.Nonzero(t, err)

	be.Equal(t, genA, s.getWorkload(lifecycleNamespace, id))
	be.Equal(t, false, processGone(pidA))
}

// The resume replay (existing=true) must adopt a running generation from
// inside the same critical section: the state layer answers with the adopted
// generation's name and must not spawn a second process.
func TestResumeWorkloadAdoptsRunningGeneration(t *testing.T) {
	s := newLifecycleState(t)
	const id = "wl-atomic-resume"

	be.NilErr(t, s.AddWorkload(lifecycleNamespace, id, serviceRequest(t, "gen-a", "30")))
	pidA := runningPid(t, s, lifecycleNamespace, id)
	genA := s.getWorkload(lifecycleNamespace, id)

	name, err := s.ResumeWorkload(lifecycleNamespace, id, serviceRequest(t, "gen-b", "30"))
	be.NilErr(t, err)
	be.Equal(t, "gen-a", name)

	be.Equal(t, genA, s.getWorkload(lifecycleNamespace, id))
	be.Equal(t, pidA, runningPid(t, s, lifecycleNamespace, id))
}

// The agent-level guard semantics are unchanged by moving them into the state
// layer: a fresh start against a running id still reports "already running".
func TestStartWorkloadStillGuardsAfterMove(t *testing.T) {
	a := newLifecycleAgent(t)
	const id = "wl-atomic-agent"

	_, err := a.StartWorkload(id, serviceRequest(t, "gen-a", "30"), false)
	be.NilErr(t, err)
	runningPid(t, a.state, lifecycleNamespace, id)

	_, err = a.StartWorkload(id, serviceRequest(t, "gen-b", "30"), false)
	be.Nonzero(t, err)

	be.NilErr(t, a.StopWorkload(id, &models.StopWorkloadRequest{Namespace: lifecycleNamespace}))
}

// The hammer: concurrent fresh starts for one id must yield exactly one
// winner, however they interleave. This pins the atomicity itself, not just
// the API boundary -- a regression that re-splits the guard from the insert
// (a pre-check under its own lock acquisition) passes the single-threaded
// tests above but loses here.
func TestAddWorkloadConcurrentSameIdOneWinner(t *testing.T) {
	s := newLifecycleState(t)
	const id = "wl-atomic-hammer"
	const racers = 8

	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := s.AddWorkload(lifecycleNamespace, id, serviceRequest(t, fmt.Sprintf("gen-%d", i), "30")); err == nil {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()

	be.Equal(t, int32(1), successes.Load())

	// Exactly one generation is tracked and its process is alive.
	pid := runningPid(t, s, lifecycleNamespace, id)
	be.Equal(t, false, processGone(pid))
	be.NilErr(t, s.RemoveWorkload(lifecycleNamespace, id))
}
