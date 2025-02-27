package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/synadia-io/nex/api"
	"golang.org/x/net/context"
)

type Monitor struct {
	Events Events `cmd:"" help:"Monitor events"`
	Logs   Logs   `cmd:"" help:"Monitor logs"`
}

// Monitor subcommands
type Events struct {
	WorkloadID string `default:"*" help:"Workload ID to filter logs"`
	EventType  string `default:"*" help:"Filter events on type"`
}

func (e *Events) Run(ctx context.Context, globals *Globals) error {
	if globals.Check {
		return printTable("Monitor Events Configuration", append(globals.Table(), e.Table()...)...)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		cancel()
	}()

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	controller, err := api.NewControlApiClient(nc, slog.New(slog.NewTextHandler(os.Stdin, nil)))
	if err != nil {
		return err
	}

	logs, err := controller.MonitorEvents(globals.Namespace, e.WorkloadID, e.EventType)
	if err != nil {
		return err
	}

	eWord := "All"
	if e.EventType != "*" {
		eWord = e.EventType
	}
	fmt.Printf("##### %s event types for application: %s\n", eWord, e.WorkloadID)

logloop:
	for {
		select {
		case <-ctx.Done():
			break logloop
		case ll := <-logs:
			data, err := ll.MarshalJSON()
			if err != nil {
				return err
			}
			fmt.Printf("[%s] -> %s\n", time.Now().Format(time.TimeOnly), string(data))
		}
	}

	return nil
}

type Logs struct {
	WorkloadID string `default:"*" help:"Workload ID to filter logs"`
	Level      string `default:"*" enum:"*,stdout,stderr" help:"Filter logs on stdout | stderr"`
}

func (l *Logs) Run(ctx context.Context, globals *Globals) error {
	if globals.Check {
		return printTable("Monitor Logs Configuration", append(globals.Table(), l.Table()...)...)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		cancel()
	}()

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	controller, err := api.NewControlApiClient(nc, slog.New(slog.NewTextHandler(os.Stdin, nil)))
	if err != nil {
		return err
	}

	logs, err := controller.MonitorLogs(globals.Namespace, l.WorkloadID, l.Level)
	if err != nil {
		return err
	}

	fmt.Printf("##### Logs for application: %s\n", l.WorkloadID)

logloop:
	for {
		select {
		case <-ctx.Done():
			break logloop
		case ll := <-logs:
			fmt.Printf("[%s] -> %s\n", time.Now().Format(time.TimeOnly), string(ll))
		}
	}

	return nil
}
