//go:build !windows

package node

import (
	"os"
	"os/signal"
	"syscall"
)

func signalReset(i chan os.Signal) {
	signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGHUP)
	signal.Notify(i, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
}
