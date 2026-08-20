//go:build linux

package native

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// processStartTime returns field 22 of /proc/<pid>/stat -- the process start
// time in clock ticks since boot. It is stable for a given process and differs
// for any process later assigned the same pid, which is the reuse guard the
// reaper needs.
//
// Field 2 (comm) is parenthesised and may itself contain spaces and
// parentheses, so the record is split at the LAST ')': the space-separated
// fields after it begin at field 3 (state), making start time (field 22) the
// element at index 19.
func processStartTime(pid int) (int64, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	s := string(data)
	close := strings.LastIndexByte(s, ')')
	if close < 0 {
		return 0, errors.New("malformed /proc stat: no comm terminator")
	}
	fields := strings.Fields(s[close+1:])
	if len(fields) < 20 {
		return 0, errors.New("malformed /proc stat: too few fields")
	}
	return strconv.ParseInt(fields[19], 10, 64)
}
