package nexagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	"github.com/ConnectEverything/nex/nex-agent/providers"
	"github.com/cloudevents/sdk-go/pkg/cloudevents"
	"github.com/nats-io/nats.go"
)

const NexAgentSubjectAdvertise = "agentint.advertise"
const workloadExecutionSleepTimeoutMillis = 1000

// Agent facilitates communication between the nex agent running in the firecracker VM
// and the nex node by way of a configured internal NATS server. Agent instances provide
// logging and event emission facilities and execute dispatched workloads
type Agent struct {
	agentLogs chan *agentapi.LogEntry
	eventLogs chan *cloudevents.Event
	connected bool
	lastError error

	cacheBucket nats.ObjectStore
	md          *agentapi.MachineMetadata
	nc          *nats.Conn
	started     time.Time
}

// NewAgent initializes a new agent to facilitate communications with
// the host node and dispatch workloads
func NewAgent() (*Agent, error) {
	metadata, err := GetMachineMetadata()
	if err != nil {
		return nil, err
	}

	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", metadata.NodeNatsAddress, metadata.NodePort))
	if err != nil {
		return nil, err
	}

	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}

	bucket, err := js.ObjectStore(agentapi.WorkloadCacheBucket)
	if err != nil {
		return nil, err
	}

	return &Agent{
		agentLogs:   make(chan *agentapi.LogEntry),
		eventLogs:   make(chan *cloudevents.Event),
		cacheBucket: bucket,
		md:          metadata,
		nc:          nc,
		started:     time.Now().UTC(),
	}, nil
}

// Start the agent
func (a *Agent) Start() error {
	err := a.Advertise()
	if err != nil {
		a.lastError = err
		return err
	}

	subject := fmt.Sprintf("agentint.%s.workdispatch", a.md.VmId)
	_, err = a.nc.Subscribe(subject, a.handleWorkDispatched)
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to subscribe to work dispatch: %s", err))
		return err
	}

	// TODO: decide whether we should have `/logz` endpoint to read from the agent's openrc-defined log files
	http.HandleFunc("/healthz", handleHealthz(a))
	go http.ListenAndServe(":9999", nil)

	go a.dispatchEvents()
	go a.dispatchLogs()

	return nil
}

// Publish an initial message to the host indicating the agent is "all the way" up
func (a *Agent) Advertise() error {
	msg := agentapi.AdvertiseMessage{
		MachineId: a.md.VmId,
		StartTime: a.started,
		Message:   a.md.Message,
	}
	raw, _ := json.Marshal(msg)

	_, err := a.nc.Request(NexAgentSubjectAdvertise, raw, 100*time.Millisecond)
	if err != nil {
		a.LogError(fmt.Sprintf("Agent failed to request initial sync message: %s", err))
		return err
	}

	a.LogInfo("Agent is up")
	a.connected = true
	return nil
}

// Pull a RunRequest off the wire, get the payload from the shared
// bucket, write it to temp, initialize the execution provider per
// the work request, and then execute it
func (a *Agent) handleWorkDispatched(m *nats.Msg) {
	var request agentapi.WorkRequest
	err := json.Unmarshal(m.Data, &request)
	if err != nil {
		msg := fmt.Sprintf("Failed to unmarshal work request: %s", err)
		a.LogError(msg)
		_ = a.workAck(m, false, msg)
		return
	}

	tmpFile, err := a.cacheExecutableArtifact(&request)
	if err != nil {
		_ = a.workAck(m, false, err.Error())
		return
	}

	params, err := a.newExecutionProviderParams(&request, *tmpFile)
	if err != nil {
		_ = a.workAck(m, false, err.Error())
		return
	}

	provider, err := providers.NewExecutionProvider(params)
	if err != nil {
		msg := fmt.Sprintf("Failed to initialize workload execution provider; %s", err)
		a.LogError(msg)
		_ = a.workAck(m, false, msg)
		return
	}

	err = provider.Validate()
	if err != nil {
		msg := fmt.Sprintf("Failed to validate workload: %s", err)
		a.LogError(msg)
		_ = a.workAck(m, false, msg)
		return
	}

	_ = a.workAck(m, true, "Workload accepted")

	err = provider.Execute()
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to execute workload: %s", err))
	}
}

// cacheExecutableArtifact uses the underlying agent configuration to fetch
// the executable workload artifact from the cache bucket, write it to a
// temporary file and make it executable; this method returns the full
// path to the cached artifact if successful
func (a *Agent) cacheExecutableArtifact(req *agentapi.WorkRequest) (*string, error) {
	tempFile := path.Join(os.TempDir(), "workload") // FIXME-- randomly generate a filename

	err := a.cacheBucket.GetFile(req.WorkloadName, tempFile)
	if err != nil {
		msg := fmt.Sprintf("Failed to write workload artifact to temp dir: %s", err)
		a.LogError(msg)
		return nil, errors.New(msg)
	}

	err = os.Chmod(tempFile, 0777)
	if err != nil {
		msg := fmt.Sprintf("Failed to set workload artifact as executable: %s", err)
		a.LogError(msg)
		return nil, errors.New(msg)
	}

	return &tempFile, nil
}

// newExecutionProviderParams initializes new execution provider params
// for the given work request and starts a goroutine listening
func (a *Agent) newExecutionProviderParams(req *agentapi.WorkRequest, tmpFile string) (*agentapi.ExecutionProviderParams, error) {
	params := &agentapi.ExecutionProviderParams{
		WorkRequest: *req,
		Stderr:      &logEmitter{stderr: true, name: req.WorkloadName, logs: a.agentLogs},
		Stdout:      &logEmitter{stderr: false, name: req.WorkloadName, logs: a.agentLogs},
		TmpFilename: tmpFile,
		VmID:        a.md.VmId,

		Fail: make(chan bool),
		Run:  make(chan bool),
		Exit: make(chan int),
	}

	go func() {
		sleepMillis := agentapi.DefaultRunloopSleepTimeoutMillis

		for {
			select {
			case <-params.Fail:
				msg := fmt.Sprintf("Failed to start workload: %s; vm: %s", params.WorkloadName, params.VmID)
				a.lastError = errors.New(msg)
				a.PublishWorkloadExited(params.VmID, params.WorkloadName, msg, true, -1)
				return

			case <-params.Run:
				a.PublishWorkloadStarted(params.VmID, params.WorkloadName, params.TotalBytes)
				sleepMillis = workloadExecutionSleepTimeoutMillis

			case exit := <-params.Exit:
				msg := fmt.Sprintf("Exited workload: %s; vm: %s; status: %d", params.WorkloadName, params.VmID, exit)
				a.PublishWorkloadExited(params.VmID, params.WorkloadName, msg, exit != 0, exit)
				return
			default:
				// no-op
			}

			time.Sleep(time.Millisecond * time.Duration(sleepMillis))
		}
	}()

	return params, nil
}

// workAck ACKs the provided NATS message by responding with the
// accepted status of the attempted work request and associated message
func (a *Agent) workAck(m *nats.Msg, accepted bool, msg string) error {
	ack := agentapi.WorkResponse{
		Accepted: accepted,
		Message:  msg,
	}

	bytes, err := json.Marshal(&ack)
	if err != nil {
		return err
	}

	err = m.Respond(bytes)
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to acknowledge work dispatch: %s", err))
		return err
	}

	return nil
}

func handleHealthz(a *Agent) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, _req *http.Request) {

		res := struct {
			Connected bool    `json:"connected"`
			Started   string  `json:"started"`
			LastError *string `json:"last_error,omitempty"`
		}{
			Connected: a.connected,
			Started:   a.started.Format(time.RFC3339),
			LastError: a.getLastError(),
		}
		bytes, _ := json.Marshal(res)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes)
	}
}

func (a *Agent) getLastError() *string {
	if a.lastError == nil {
		return nil
	}
	s := a.lastError.Error()
	return &s
}
