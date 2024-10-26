package node

import (
	"os"
	"os/signal"
	"syscall"
)

func signalReset(i chan os.Signal) {
	signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	signal.Notify(i, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
}
