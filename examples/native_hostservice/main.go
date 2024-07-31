package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
)

func main() {
	server, found := os.LookupEnv("NEX_HOSTSERVICES_NATS_SERVER")
	if !found {
		fmt.Fprintln(os.Stderr, "failed to find NEX_HOSTSERVICES_NATS_SERVER in environment")
		return
	}
	jwt, found := os.LookupEnv("NEX_HOSTSERVICES_NATS_USER_JWT")
	if !found {
		fmt.Fprintln(os.Stderr, "failed to find NEX_HOSTSERVICES_NATS_USER_JWT in environment")
		return
	}
	seed, found := os.LookupEnv("NEX_HOSTSERVICES_NATS_USER_SEED")
	if !found {
		fmt.Fprintln(os.Stderr, "failed to find NEX_HOSTSERVICES_NATS_USER_SEED in environment")
		return
	}

	fmt.Fprintf(os.Stdout, "Found environment!\nServer: %s\nJWT: %s\nSeed: %s\n", server, jwt, seed)

	// You would be able to use the JWT and Seed to connect to the NATS server, but since they are
	// fake here, we will just print them out and connect to demo.nats.io

	ctx := context.Background()
	nc, err := nats.Connect(server)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to NATS: %s\n", err)
		return
	}
	setupSignalHandlers(nc)

	_, err = nc.Subscribe("fundemo", func(msg *nats.Msg) {
		fmt.Fprintf(os.Stdout, "Received message: %s\n", string(msg.Data))
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to subscribe to fundemo: %s\n", err)
		return
	}

	<-ctx.Done()
}

func setupSignalHandlers(nc *nats.Conn) {
	go func() {
		signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt || s == syscall.SIGQUIT:
				fmt.Fprintf(os.Stdout, "Caught signal [%s], requesting clean shutdown", s.String())
				nc.Drain()
				os.Exit(0)

			default:
				nc.Drain()
				os.Exit(0)
			}
		}
	}()
}
