package nexagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	"github.com/cloudevents/sdk-go/pkg/cloudevents"
	"github.com/nats-io/nats.go"
)

// Anywhere within agent code that needs to dispatch an event or a log to the host
// node server, it can do so by putting a message on the appropriate channel
var (
	agentLogs chan (*agentapi.LogEntry) = make(chan *agentapi.LogEntry)
	eventLogs chan (cloudevents.Event)  = make(chan cloudevents.Event)
)

const (
	AdvertiseSubject = "agentint.advertise"
	MyWorkloadType   = "elf"
)

type NodeClient struct {
	nc          *nats.Conn
	md          *agentapi.MachineMetadata
	cacheBucket nats.ObjectStore
	started     time.Time
}

// Creates a new client to communicate with the host node
func NewNodeClient(nc *nats.Conn, md *agentapi.MachineMetadata) (*NodeClient, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	bucket, err := js.ObjectStore(agentapi.WorkloadCacheBucket)
	if err != nil {
		return nil, err
	}

	return &NodeClient{
		nc:          nc,
		md:          md,
		cacheBucket: bucket,
		started:     time.Now().UTC(),
	}, nil
}

func (node *NodeClient) Start() {
	node.Advertise()

	subject := fmt.Sprintf("agentint.%s.workdispatch", node.md.VmId)
	_, err := node.nc.Subscribe(subject, handleWorkDispatched(node))
	if err != nil {
		LogError(fmt.Sprintf("Failed to subscribe to work dispatch: %s", err))
		// if the agent can't subscribe to work dispatch, the agent/VM is useless
		HaltVM(err)
	}

	go dispatchEvents(node)
	go dispatchLogs(node)
}

// Publish an initial message to the host indicating the agent is "all the way" up
func (node *NodeClient) Advertise() {
	msg := agentapi.AdvertiseMessage{
		MachineId: node.md.VmId,
		StartTime: node.started,
		Message:   node.md.Message,
	}
	raw, _ := json.Marshal(msg)
	err := node.nc.Publish(AdvertiseSubject, raw)
	if err != nil {
		LogError(fmt.Sprintf("Failed to publish agent advertisement: %s", err))
	}
	node.nc.Flush()

	LogInfo("Agent is up")
}

// Pull a RunRequest off the wire, get the payload from the shared
// bucket, write it to temp, and then execute it
func handleWorkDispatched(node *NodeClient) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		var request agentapi.WorkRequest
		err := json.Unmarshal(m.Data, &request)
		if err != nil {
			msg := fmt.Sprintf("Failed to unmarshal work request: %s", err)
			LogError(msg)
			workAck(m, false, msg)
			return
		}
		if request.WorkloadType != MyWorkloadType {
			// silently do nothing, allowing other potential agents that do
			// handle that workload to respond
			LogDebug(fmt.Sprintf("Ignoring request to start workload of type '%s'", request.WorkloadType))
			return
		}

		tempFile := path.Join(os.TempDir(), "workload")
		err = node.cacheBucket.GetFile(request.WorkloadName, tempFile)
		if err != nil {
			msg := fmt.Sprintf("Failed to write workload to temp dir: %s", err)
			LogError(msg)
			workAck(m, false, msg)
			return
		}
		err = os.Chmod(tempFile, 0777)
		if err != nil {
			LogError(fmt.Sprintf("Failed to set workload as executable: %s", err))
			return
		}

		workAck(m, true, "Workload accepted")
		err = RunWorkload(node.md.VmId, request.WorkloadName, int32(request.TotalBytes), tempFile, request.Environment)
		if err != nil {
			LogError(fmt.Sprintf("Failed to run workload: %s", err))
		}
	}
}

func workAck(m *nats.Msg, accepted bool, msg string) {
	ack := agentapi.WorkResponse{
		Accepted: accepted,
		Message:  msg,
	}
	bytes, err := json.Marshal(&ack)
	if err == nil {
		err = m.Respond(bytes)
		if err != nil {
			LogError(fmt.Sprintf("Failed to acknowledge work dispatch: %s", err))
		}
	}
}

// This is run inside a goroutine to pull log entries off the channel and publish to the
// node host via internal NATS
func dispatchLogs(node *NodeClient) {
	for {
		entry := <-agentLogs
		bytes, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		subject := fmt.Sprintf("agentint.%s.logs", node.md.VmId)
		err = node.nc.Publish(subject, bytes)
		if err != nil {
			continue
		}
		node.nc.Flush()
	}
}

// Run inside a goroutine to pull event entries and publish them to the node host.
func dispatchEvents(node *NodeClient) {
	for {
		entry := <-eventLogs
		bytes, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		subject := fmt.Sprintf("agentint.%s.events.%s", node.md.VmId, entry.Type())
		err = node.nc.Publish(subject, bytes)

		if err != nil {
			continue
		}
		node.nc.Flush()
	}
}
