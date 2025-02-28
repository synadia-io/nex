//go:build !windows

package native

func getFileName() string {
	return "workload-*"
}
