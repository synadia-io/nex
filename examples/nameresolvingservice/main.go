package main

import (
	"context"
	"fmt"
	"net"
	"os"
)

var (
	cancelF context.CancelFunc
	closing uint32
	ctx     context.Context
	sigs    chan os.Signal
)

func main() {
	// ctx = context.Background()
	// installSignalHandlers()

	hostname := "google.com"

	ips, err := net.LookupIP(hostname)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve hostname: %s; %s\n", hostname, err.Error())
		os.Exit(1)
	}

	for _, ip := range ips {
		fmt.Printf("%s. IN A %s\n", hostname, ip.String())
	}
}

// func installSignalHandlers() {
// 	signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
// 	sigs = make(chan os.Signal, 1)
// 	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
// }

// func shutdown() {
// 	if atomic.AddUint32(&closing, 1) == 1 {
// 		signal.Stop(sigs)
// 		close(sigs)
// 	}
// }
