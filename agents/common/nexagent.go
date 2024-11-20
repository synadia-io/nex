package agentcommon

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
}

// TODO: these callbacks need appropriate parameters
type AgentCallback interface {
	Up() error
	Preflight() error
	StartWorkload(request *agentapigen.StartWorkloadRequestJson) error
	StopWorkload(request *agentapigen.StopWorkloadRequestJson) error
	ListWorkloads() error
	Trigger(workloadId string, data []byte) error
}

func NewNexAgent(name, version, description string,
	maxWorkloads int, callback AgentCallback) (*NexAgent, error) {

	fmt.Fprintf(os.Stdout, "Starting Nex Agent %s (v%s)\n", name, version)

	embedded, err := CreateEmbeddedNatsConnection(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create connection to NATS server: %s\n", err)
		return nil, err
	}
	return &NexAgent{
		embeddedNats: embedded,
		callback:     callback,
		name:         name,
		version:      version,
		description:  description,
		maxWorkloads: maxWorkloads,
	}, nil
}

func (agent *NexAgent) Run() error {
	if err := agent.registerWithNode(); err != nil {
		return err
	}

	if err := agent.createSubscriptions(); err != nil {
		return err
	}

	switch os.Args[1] {
	case "up":
		return agent.callback.Up()
	case "preflight":
		return agent.callback.Preflight()
	default:
		fmt.Println("No command supplied")
	}
	return nil
}

func (agent *NexAgent) NewHostServicesConnection(workloadId string, host string, port int, jwt string, seed string) (*nats.Conn, error) {
	return createHostServicesConnection(workloadId, host, port, jwt, seed)
}

func (agent *NexAgent) registerWithNode() error {
	fmt.Fprintln(os.Stdout, "Registering agent with node")

	req := agentapigen.RegisterAgentRequestJson{
		Description:  &agent.description,
		MaxWorkloads: &agent.maxWorkloads,
		Name:         agent.name,
		Version:      agent.version,
	}
	subject := agentapi.AgentRegisterSubject()
	bytes, err := json.Marshal(&req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to marshal register request")
		return err
	}

	// TODO: right now we don't use the response type. Keep an eye on this to add a response
	// payload if necessary

	_, err = agent.embeddedNats.Request(subject, bytes, 1*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register agent with node: %s (%s)\n",
			err,
			subject)
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

	_, err = agent.embeddedNats.Subscribe(
		agentapi.WorkloadTriggerSubscribeSubject(agent.name),
		agent.handleTrigger)
	if err != nil {
		return err
	}

	return nil
}

func (agent *NexAgent) handleStartWorkload(m *nats.Msg) {
	var req agentapigen.StartWorkloadRequestJson
	err := json.Unmarshal(m.Data, &req)
	if err != nil {
		_ = 0 // for linter
		// TODO: return error envelope
	}
	err = agent.callback.StartWorkload(&req)
	if err != nil {
		_ = 0 // for linter
		// TODO: return error envelope
	}
	// TODO: return success envelope
}

func (agent *NexAgent) handleTrigger(m *nats.Msg) {
	tokens := strings.Split(m.Subject, ".")
	// agent.{workload type}.workloads.{workload id}.trigger
	workloadId := tokens[3]

	err := agent.callback.Trigger(workloadId, m.Data)
	if err != nil {
		_ = 0
		// TODO: return error envelope
	}
	// TODO: return success envelope
}

func (agent *NexAgent) handleStopWorkload(m *nats.Msg) {
	var req agentapigen.StopWorkloadRequestJson
	err := json.Unmarshal(m.Data, &req)
	if err != nil {
		_ = 0 // for linter
		// TODO: return error envelope
	}
	err = agent.callback.StopWorkload(&req)
	if err != nil {
		_ = 0 // for linter
		// TODO: return error envelope
	}
	// TODO: return success envelope
}

func (agent *NexAgent) handleListWorkloads(m *nats.Msg) {
	_ = agent.callback.ListWorkloads()
}

// TODO: this should actually get passed the node configuration options to make
// the preflight check accurate
type CommandHandler func() error
