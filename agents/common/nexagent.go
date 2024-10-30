package agentcommon

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/api/agent/go"
	agentapigen "github.com/synadia-io/nex/api/agent/go/gen"
)

// A convenient bundling for common functions that belong to a Nex Agent
type NexAgent struct {
	embeddedNats *nats.Conn
	callback     AgentCallback

	name         string
	version      string
	description  string
	maxWorkloads int

	logger *slog.Logger
}

// TODO: these callbacks need appropriate parameters
type AgentCallback interface {
	Up() error
	Preflight() error
	StartWorkload(request *agentapigen.StartWorkloadRequestJson) error
	StopWorkload(request *agentapigen.StopWorkloadRequestJson) error
	ListWorkloads() error
}

func NewNexAgent(name, version, description string,
	maxWorkloads int, logger *slog.Logger, callback AgentCallback) (*NexAgent, error) {

	// TODO: generate whatever context we need

	embedded, err := CreateEmbeddedNatsConnection()
	if err != nil {
		return nil, err
	}
	return &NexAgent{
		logger:       logger,
		embeddedNats: embedded,
		callback:     callback,
		name:         name,
		version:      version,
		description:  description,
		maxWorkloads: maxWorkloads,
	}, nil
}

func (agent *NexAgent) Run() error {
	// TODO: based on argv, decide which handler to call
	// get error from handler and return here

	if err := agent.registerWithNode(); err != nil {
		return err
	}

	if err := agent.createSubscriptions(); err != nil {
		return err
	}

	switch os.Args[1] {
	case "up":
		agent.callback.Up()
	case "preflight":
		agent.callback.Preflight()
	default:
		fmt.Println("No command supplied")
	}
	return nil
}

func (agent *NexAgent) NewHostServicesConnection(workloadId string, host string, port int, jwt string, seed string) (*nats.Conn, error) {
	return createHostServicesConnection(workloadId, host, port, jwt, seed)
}

func (agent *NexAgent) registerWithNode() error {
	agent.logger.Info("Registering agent with node", slog.String("name", agent.name))

	req := agentapigen.RegisterAgentRequestJson{
		Description:  &agent.description,
		MaxWorkloads: &agent.maxWorkloads,
		Name:         agent.name,
		Version:      agent.version,
	}
	subject := agentapi.AgentRegisterSubject(agent.name)
	bytes, err := json.Marshal(&req)
	if err != nil {
		agent.logger.Error("Failed to register agent with node", slog.Any("error", err))
		return err
	}

	// TODO: right now we don't use the response type. Keep an eye on this to add a response
	// payload if necessary

	_, err = agent.embeddedNats.Request(subject, bytes, 1*time.Second)
	if err != nil {
		return err
	}

	return nil
}

func (agent *NexAgent) createSubscriptions() error {

	_, err := agent.embeddedNats.Subscribe(
		agentapi.StartWorkloadSubscribeSubject(agent.name),
		agent.handleStartWorkload)
	if err != nil {
		return err
	}

	_, err = agent.embeddedNats.Subscribe(
		agentapi.StopWorkloadSubscribeSubject(agent.name),
		agent.handleStopWorkload)
	if err != nil {
		return err
	}

	_, err = agent.embeddedNats.Subscribe(
		agentapi.ListWorkloadsSubscribeSubject(agent.name),
		agent.handleListWorkloads)
	if err != nil {
		return err
	}

	return nil
}

func (agent *NexAgent) handleStartWorkload(m *nats.Msg) {
	var req agentapigen.StartWorkloadRequestJson
	err := json.Unmarshal(m.Data, &req)
	if err != nil {
		// TODO: return error envelope
	}
	err = agent.callback.StartWorkload(&req)
	if err != nil {
		// TODO: return error envelope
	}
	// TODO: return success envelope
}

func (agent *NexAgent) handleStopWorkload(m *nats.Msg) {
	var req agentapigen.StopWorkloadRequestJson
	err := json.Unmarshal(m.Data, &req)
	if err != nil {
		// TODO: return error envelope
	}
	err = agent.callback.StopWorkload(&req)
	if err != nil {
		// TODO: return error envelope
	}
	// TODO: return success envelope
}

func (agent *NexAgent) handleListWorkloads(m *nats.Msg) {
	agent.callback.ListWorkloads()
}

// TODO: this should actually get passed the node configuration options to make
// the preflight check accurate
type CommandHandler func() error
