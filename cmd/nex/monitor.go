package main

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/models"
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

	subject := models.EVENTS_SUBJECT
	f_subject, err := subject.Filter(e.WorkloadID, e.EventType)
	if err != nil {
		return err
	}

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	startTime := time.Now()
	sub, err := nc.Subscribe(f_subject, func(msg *nats.Msg) {
		fmt.Printf("%s [%.1fs] -> %s\n", msg.Subject, time.Since(startTime).Seconds(), msg.Data)
	})
	if err != nil {
		return err
	}

	<-ctx.Done()

	return sub.Drain()
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

	subject := models.LOGS_SUBJECT
	f_subject, err := subject.Filter(l.WorkloadID, l.Level)
	if err != nil {
		return err
	}

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	startTime := time.Now()
	sub, err := nc.Subscribe(f_subject, func(msg *nats.Msg) {
		fmt.Printf("%s [%.1fs] -> %s\n", msg.Subject, time.Since(startTime).Seconds(), msg.Data)
	})
	if err != nil {
		return err
	}

	<-ctx.Done()

	return sub.Drain()
}
