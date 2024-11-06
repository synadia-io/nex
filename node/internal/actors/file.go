//go:build !windows

package actors

func getFileName() string {
	return "workload-*"
}
