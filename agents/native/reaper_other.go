//go:build !linux && !darwin

package native

import "errors"

// processStartTime has no cheap, portable source on other platforms (e.g.
// Windows), so it reports the start time as unavailable. That disables the
// reaper there -- record and reapOrphans become no-ops -- rather than reaping
// without a pid-reuse guard, which would be unsafe.
func processStartTime(pid int) (int64, error) {
	return 0, errors.New("process start time not available on this platform")
}
