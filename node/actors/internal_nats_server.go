package actors

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
)

func createInternalNatsServer() gen.ProcessBehavior {
	return &internalNatsServer{}
}

type internalNatsServer struct {
	act.Actor
	tokens        map[gen.Atom]gen.Ref
	haveConsumers bool

	creds       []agentCredential
	nodeOptions models.NodeOptions
}

type agentCredential struct {
	workloadType string
	nkey         nkeys.KeyPair
}

func (ns *internalNatsServer) Init(args ...any) error {
	ns.tokens = make(map[gen.Atom]gen.Ref)

	ns.Log().Info("Internal NATS server started")

	ns.nodeOptions = args[0].(models.NodeOptions)

	creds, err := ns.buildAgentCredentials()
	if err != nil {
		return err
	}

	ns.creds = creds
	err = ns.startNatsServer(creds)
	if err != nil {
		return err
	}

	err = ns.Send(ns.PID(), PostInit)
	if err != nil {
		return err
	}

	return nil
}

func (ns *internalNatsServer) HandleMessage(from gen.PID, message any) error {
	eventStart := gen.MessageEventStart{Name: InternalNatsServerReadyName}
	eventStop := gen.MessageEventStop{Name: InternalNatsServerReadyName}

	switch message {
	case PostInit:
		evOptions := gen.EventOptions{
			// NOTE: notify true allows us to deterministically wait until we have
			// a consumer before we publish an event. No more sleep-and-hope pattern.
			Notify: true,
		}
		token, err := ns.RegisterEvent(InternalNatsServerReadyName, evOptions)
		if err != nil {
			return err
		}
		ns.tokens[InternalNatsServerReadyName] = token
		ns.Log().Info("registered publishable event %s, waiting for consumers...", InternalNatsServerReadyName)
	case eventStart:
		ns.Log().Info("publisher got first consumer for %s. start producing events...", InternalNatsServerReadyName)
		ns.haveConsumers = true
		err := ns.SendEvent(InternalNatsServerReadyName, ns.tokens[InternalNatsServerReadyName], InternalNatsServerReadyEvent{AgentCredentials: ns.creds})
		if err != nil {
			return err
		}
	case eventStop: // handle gen.MessageEventStop message
		ns.Log().Info("no consumers for %s", InternalNatsServerReadyName)
		ns.haveConsumers = false
	}
	return nil
}

// HandleInspect invoked on the request made with gen.Process.Inspect(...)
func (ns *internalNatsServer) HandleInspect(from gen.PID, item ...string) map[string]string {
	ns.Log().Info("internal nats server got inspect request from %s", from)
	return nil
}

func (ns *internalNatsServer) buildAgentCredentials() ([]agentCredential, error) {
	creds := make([]agentCredential, len(ns.nodeOptions.WorkloadOptions))
	for i, w := range ns.nodeOptions.WorkloadOptions {
		kp, _ := nkeys.CreateUser()
		creds[i] = agentCredential{
			workloadType: w.Name,
			nkey:         kp,
		}
	}
	return creds, nil
}

func (ns *internalNatsServer) startNatsServer(creds []agentCredential) error {
	ns.Log().Debug("Starting internal NATS server")
	// TODO!
	return nil
}
