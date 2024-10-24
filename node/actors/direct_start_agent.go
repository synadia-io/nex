package actors

import (
	"fmt"
	"log/slog"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func createDirectStartAgent() gen.ProcessBehavior {
	return &directStartAgent{}
}

type directStartAgent struct {
	act.Actor
}

func (a *directStartAgent) Init(args ...any) error {
	return nil
}

func (a *directStartAgent) HandleMessage(from gen.PID, message any) error {
	// POST INIT -> start binary
	// if err -> panic nex
	a.Log().Info("info", slog.Any("message", message))
	a.Log().Debug("debug", slog.Any("message", message))
	fmt.Printf("%#v\n", message)
	return nil
}
