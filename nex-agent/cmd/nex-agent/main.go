//go:build linux

package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"

	nexagent "github.com/ConnectEverything/nex/nex-agent"
)

func main() {
	logFile, err := os.Create("/home/nex/agent.log")
	if err != nil {
		haltVM(err)
	}
	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)

	ctx := context.Background()

	agent, err := nexagent.NewAgent()
	if err != nil {
		haltVM(err)
	}

	if agent != nil {
		err = agent.Start()
		if err != nil {
			haltVM(err)
		}
	}

	signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGHUP)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	go func() {
		signal := <-c
		cleanup(signal, logFile)
		os.Exit(1)
	}()

	<-ctx.Done()
}

func cleanup(c os.Signal, logFile *os.File) {
	log.WithField("signal", c).Info("Agent terminating")
	_ = logFile.Close()

}

// haltVM stops the firecracker VM
func haltVM(err error) {
	// On the off chance the agent's log is captured from the vm
	log.WithError(err).Error("Terminating Firecracker VM due to fatal error")
	err = syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
	if err != nil {
		log.WithError(err).Error("Failed to request reboot")
	}
}
