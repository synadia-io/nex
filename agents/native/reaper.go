package native

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/synadia-io/nex/internal"
)

// orphanReaper records the pids of workloads this node has spawned, with each
// process's start time, in a node-local file. After a HARD node crash (SIGKILL,
// OOM) the workload processes are orphaned rather than stopped; on the next
// startup reapOrphans kills any survivors BEFORE resume-on-registration can
// re-create them, which would otherwise run two copies of the same workload.
//
// The recorded start time is the pid-reuse guard. A pid recorded before the
// crash can be recycled by the OS afterwards, and a blind kill(-pid) would then
// group-kill an unrelated process (every shell job is a group leader). So a
// leftover pid is killed only when the live process at that pid still reports
// the start time we recorded for it.
//
// A reaper with no path (no resource directory available, e.g. tests) is
// disabled and every method is a no-op.
type orphanReaper struct {
	logger *slog.Logger
	path   string
	mu     sync.Mutex
}

// newOrphanReaper builds a reaper writing to "<resourceDir>/<nodeID>.workload-pids".
// An empty resourceDir, or a directory that cannot be created, yields a
// disabled (no-op) reaper rather than an error -- the node still runs, it just
// cannot reap crash orphans.
func newOrphanReaper(resourceDir, nodeID string, logger *slog.Logger) *orphanReaper {
	r := &orphanReaper{logger: logger}
	if resourceDir == "" {
		return r
	}
	if err := os.MkdirAll(resourceDir, 0o700); err != nil {
		logger.Warn("orphan reaper disabled: cannot create resource directory", slog.String("dir", resourceDir), slog.String("err", err.Error()))
		return r
	}
	r.path = filepath.Join(resourceDir, nodeID+".workload-pids")
	return r
}

type pidRecord struct {
	pid        int
	createTime int64
}

// processCreateTime returns the process start time (ms since epoch) reported by
// the OS, which together with the pid is a stable identity that survives pid
// reuse: a recycled pid belongs to a process with a different start time.
func processCreateTime(pid int) (int64, error) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return 0, err
	}
	return p.CreateTime()
}

// record notes a freshly spawned workload's pid and start time. If the start
// time cannot be read there is no safe reuse guard, so the pid is NOT recorded
// (better to miss a reap than to risk killing a reused pid later).
func (r *orphanReaper) record(pid int) {
	if r == nil || r.path == "" {
		return
	}
	ct, err := processCreateTime(pid)
	if err != nil {
		r.logger.Warn("orphan reaper: could not read process start time; not recording pid", slog.Int("pid", pid), slog.String("err", err.Error()))
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		r.logger.Warn("orphan reaper: could not open pid file to record", slog.String("err", err.Error()))
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := fmt.Fprintf(f, "%d %d\n", pid, ct); err != nil {
		r.logger.Warn("orphan reaper: could not write pid record", slog.String("err", err.Error()))
	}
}

// forget removes a pid's record once its process has exited, so the file stays
// bounded across the node's lifetime and holds only still-running workloads.
func (r *orphanReaper) forget(pid int) {
	if r == nil || r.path == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	records, err := r.readLocked()
	if err != nil {
		return
	}
	kept := records[:0]
	for _, rec := range records {
		if rec.pid != pid {
			kept = append(kept, rec)
		}
	}
	r.writeLocked(kept)
}

// reapOrphans kills any recorded pid whose live process still matches the
// recorded start time, then clears the file. Runs once at startup, before
// resume, so a crash orphan is gone before its stored definition is replayed.
func (r *orphanReaper) reapOrphans() {
	if r == nil || r.path == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	records, err := r.readLocked()
	if err != nil {
		if !os.IsNotExist(err) {
			r.logger.Warn("orphan reaper: could not read pid file at startup", slog.String("err", err.Error()))
		}
		return
	}

	for _, rec := range records {
		ct, err := processCreateTime(rec.pid)
		if err != nil {
			continue // process already gone -- nothing to reap
		}
		if ct != rec.createTime {
			continue // pid recycled by an unrelated process -- must not kill it
		}
		r.logger.Warn("reaping orphaned workload process from a previous node incarnation", slog.Int("pid", rec.pid))
		if p, ferr := os.FindProcess(rec.pid); ferr == nil {
			// KillProcess group-kills on unix (the workload leads its own
			// group); the start-time match above makes -pid safe to signal.
			if kerr := internal.KillProcess(p); kerr != nil {
				r.logger.Warn("orphan reaper: kill failed", slog.Int("pid", rec.pid), slog.String("err", kerr.Error()))
			}
		}
	}

	if err := os.Remove(r.path); err != nil && !os.IsNotExist(err) {
		r.logger.Warn("orphan reaper: could not clear pid file", slog.String("err", err.Error()))
	}
}

// readLocked parses the pid file; malformed lines are skipped rather than
// failing the whole read, so a torn write from a crash mid-append cannot block
// reaping the records that did land.
func (r *orphanReaper) readLocked() ([]pidRecord, error) {
	f, err := os.Open(r.path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []pidRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		pid, perr := strconv.Atoi(fields[0])
		ct, cerr := strconv.ParseInt(fields[1], 10, 64)
		if perr != nil || cerr != nil || pid <= 0 {
			continue
		}
		out = append(out, pidRecord{pid: pid, createTime: ct})
	}
	return out, sc.Err()
}

// writeLocked rewrites the pid file via a temp file + rename so a crash during
// the rewrite leaves the previous file intact rather than a truncated one.
func (r *orphanReaper) writeLocked(records []pidRecord) {
	tmp, err := os.CreateTemp(filepath.Dir(r.path), filepath.Base(r.path)+".*")
	if err != nil {
		r.logger.Warn("orphan reaper: could not create temp pid file", slog.String("err", err.Error()))
		return
	}
	tmpName := tmp.Name()
	for _, rec := range records {
		if _, err := fmt.Fprintf(tmp, "%d %d\n", rec.pid, rec.createTime); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
			r.logger.Warn("orphan reaper: could not write temp pid file", slog.String("err", err.Error()))
			return
		}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, r.path); err != nil {
		_ = os.Remove(tmpName)
		r.logger.Warn("orphan reaper: could not replace pid file", slog.String("err", err.Error()))
	}
}
