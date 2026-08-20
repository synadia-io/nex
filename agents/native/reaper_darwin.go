//go:build darwin

package native

import "golang.org/x/sys/unix"

// processStartTime returns the process start time (kinfo_proc p_starttime) in
// microseconds since the epoch. It is stable for a given process and differs
// for any process later assigned the same pid -- the reuse guard the reaper
// needs.
func processStartTime(pid int) (int64, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, err
	}
	tv := kp.Proc.P_starttime
	return int64(tv.Sec)*1_000_000 + int64(tv.Usec), nil
}
