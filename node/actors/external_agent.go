package actors

import (
	"fmt"
	"log/slog"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/synadia-io/nex/models"
)

type externalAgent struct {
	act.Actor

	internalNatsCreds agentCredential
}

func createExternalAgent() gen.ProcessBehavior {
	return &externalAgent{}
}

type externalAgentParams struct {
	agentOptions models.AgentOptions
}

func (a *externalAgent) Init(args ...any) error {
	err := a.Send(a.PID(), PostInit)
	if err != nil {
		return err
	}
	return nil
}

func (a *externalAgent) HandleMessage(from gen.PID, message any) error {
	switch message {
	case PostInit:
		if _, err := a.MonitorEvent(InternalNatsServerReady); err != nil {
			fmt.Println("failed to monitor InternalNatsServerReady")
			a.Log().Error("failed to monitor InternalNatsServerReady event")
			return gen.TerminateReasonPanic
		}

		creds, err := a.Call(gen.Atom(actorNameInternalNATSServer), "imready")
		if err != nil {
			a.Log().Error("failed to call internal nats server", slog.Any("err", err))
			return gen.TerminateReasonPanic
		}

		var ok bool
		a.internalNatsCreds, ok = creds.(agentCredential)
		if !ok {
			a.Log().Error("failed to cast creds to agentCredential")
			return gen.TerminateReasonPanic
		}

		a.Log().Trace("internal nats server credentials", slog.Any("creds", creds))
	}
	return nil
}

func (a *externalAgent) HandleEvent(event gen.MessageEvent) error {
	switch event.Event.Name {
	case InternalNatsServerReadyName:
		return a.StartAgent()
	}
	return nil
}

func (a *externalAgent) StartAgent() error {
	return nil
}
