//go:build windows

package nexnode

import (
	"os"
	"os/signal"
	"syscall"
)

func (n *Node) installSignalHandlers() {
	n.log.Debug("installing signal handlers")
	// the embedded NATS server(s) register signal handlers... wipe those so ours are the ones being used
	signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	n.sigs = make(chan os.Signal, 1)
	signal.Notify(n.sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
}
