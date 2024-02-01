package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nats-io/nats.go"
	services "github.com/nats-io/nats.go/micro"
)

func main() {
	ctx := context.Background()

	natsUrl := os.Getenv("NATS_URL")
	if len(strings.TrimSpace(natsUrl)) == 0 {
		natsUrl = nats.DefaultURL
	}

	fmt.Fprintf(os.Stdout, "Echo service using NATS url '%s'\n", natsUrl)
	nc, err := nats.Connect(natsUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to NATs server: %v\n", err)
		return
	}
	setupSignalHandlers(nc)

	// request handler
	echoHandler := func(req services.Request) {
		err := req.Respond(req.Data())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error responding to request: %v\n", err)
		}
	}

	fmt.Fprint(os.Stdout, "Starting echo service")
	_, err = services.AddService(nc, services.Config{
		Name:    "EchoService",
		Version: "1.0.0",
		// base handler
		Endpoint: &services.EndpointConfig{
			Subject: "svc.echo",
			Handler: services.HandlerFunc(echoHandler),
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error adding service: %v\n", err)
		return
	}

	<-ctx.Done()

}

func setupSignalHandlers(nc *nats.Conn) {
	go func() {
		signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGHUP)
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
