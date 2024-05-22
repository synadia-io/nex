package nexagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cloudevents/sdk-go/pkg/cloudevents"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/agent/providers"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/synadia-io/nex/internal/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const defaultAgentHandshakeTimeoutMillis = 500
const runloopSleepInterval = 250 * time.Millisecond
const runloopTickInterval = 2500 * time.Millisecond
const workloadExecutionSleepTimeoutMillis = 1000

// Agent facilitates communication between the nex agent running in the firecracker VM
// and the nex node by way of a configured internal NATS server. Agent instances provide
// logging and event emission facilities, and deployment and execution of workloads
type Agent struct {
	agentLogs chan *agentapi.LogEntry
	eventLogs chan *cloudevents.Event

	cancelF context.CancelFunc
	closing uint32
	ctx     context.Context
	sigs    chan os.Signal

	provider providers.ExecutionProvider

	cacheBucket nats.ObjectStore
	md          *agentapi.MachineMetadata
	nc          *nats.Conn
	started     time.Time

	sandboxed bool
}

// Initialize a new agent to facilitate communications with the host
func NewAgent(ctx context.Context, cancelF context.CancelFunc) (*Agent, error) {
	var metadata *agentapi.MachineMetadata
	var err error

	if !isSandboxed() {
		metadata, err = GetMachineMetadataFromEnv()
	} else {
		metadata, err = GetMachineMetadata()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get machien metadata: %s\n", err)
		return nil, fmt.Errorf("failed to get machine metadata: %s", err)
	}

	if !metadata.Validate() {
		fmt.Fprintf(os.Stderr, "invalid metadata: %v\n", metadata.Errors)
		return nil, fmt.Errorf("invalid metadata: %v", metadata.Errors)
	}

	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", *metadata.NodeNatsHost, *metadata.NodeNatsPort))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to shared NATS: %s", err)
		return nil, err
	}

	js, err := nc.JetStream()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get JetStream context from shared NATS: %s", err)
		return nil, err
	}

	bucket, err := js.ObjectStore(agentapi.WorkloadCacheBucket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get reference to shared object store: %s", err)
		return nil, err
	}

	return &Agent{
		agentLogs: make(chan *agentapi.LogEntry, 64),
		eventLogs: make(chan *cloudevents.Event, 64),
		// sandbox defaults to true, only way to override that is with an explicit 'false'
		cancelF:     cancelF,
		ctx:         ctx,
		sandboxed:   isSandboxed(),
		cacheBucket: bucket,
		md:          metadata,
		nc:          nc,
		started:     time.Now().UTC(),
	}, nil
}

func (a *Agent) FullVersion() string {
	return fmt.Sprintf("%s [%s] BuildDate: %s", VERSION, COMMIT, BUILDDATE)
}

// Start the agent
// NOTE: agent process will request vm shutdown if this fails
func (a *Agent) Start() {
	if !a.sandboxed {
		a.LogDebug(fmt.Sprintf("Agent process running outside of sandbox; pid: %d", os.Getpid()))
	}

	err := a.init()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize agent: %s\n", err)
		a.LogError(fmt.Sprintf("Agent process failed to initialize; %s", err.Error()))
		a.shutdown()
	}

	timer := time.NewTicker(runloopTickInterval)
	defer timer.Stop()

	for !a.shuttingDown() {
		select {
		case <-timer.C:
			// TODO: check NATS subscription statuses, etc.
		case sig := <-a.sigs:
			a.LogInfo(fmt.Sprintf("Received signal: %s", sig))
			a.shutdown()
		case <-a.ctx.Done():
			a.shutdown()
		default:
			time.Sleep(runloopSleepInterval)
		}
	}

	a.cancelF()
}

// Request a handshake with the host indicating the agent is "all the way" up
// NOTE: the agent process will request a VM shutdown if this fails
func (a *Agent) requestHandshake() error {
	a.LogInfo("Requesting handshake from host")
	msg := agentapi.HandshakeRequest{
		ID:        a.md.VmID,
		StartTime: a.started,
		Message:   a.md.Message,
	}
	raw, _ := json.Marshal(msg)

	resp, err := a.nc.Request(fmt.Sprintf("agentint.%s.handshake", *a.md.VmID), raw, time.Millisecond*defaultAgentHandshakeTimeoutMillis)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			time.Sleep(time.Millisecond * 50)
			resp, err = a.nc.Request(fmt.Sprintf("agentint.%s.handshake", *a.md.VmID), raw, time.Millisecond*defaultAgentHandshakeTimeoutMillis)
		}

		if err != nil {
			a.LogError(fmt.Sprintf("Agent failed to request initial sync message: %s", err))
			return err
		}
	}

	var handshakeResponse *agentapi.HandshakeResponse
	err = json.Unmarshal(resp.Data, &handshakeResponse)
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to parse handshake response: %s", err))
		return err
	}

	a.LogInfo("Agent is up")
	return nil
}

func (a *Agent) Version() string {
	return VERSION
}

// cacheExecutableArtifact uses the underlying agent configuration to fetch
// the executable workload artifact from the cache bucket, write it to a
// temporary file and make it executable; this method returns the full
// path to the cached artifact if successful
func (a *Agent) cacheExecutableArtifact(req *agentapi.DeployRequest) (*string, error) {
	fileName := fmt.Sprintf("workload-%s", *a.md.VmID)
	tempFile := path.Join(os.TempDir(), fileName)

	if strings.EqualFold(runtime.GOOS, "windows") && req.WorkloadType == models.NexExecutionProviderNative {
		tempFile = fmt.Sprintf("%s.exe", tempFile)
	}

	err := a.cacheBucket.GetFile(*req.WorkloadName, tempFile)
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

// Run inside a goroutine to pull event entries and publish them to the node host.
func (a *Agent) dispatchEvents() {
	for !a.shuttingDown() {
		entry := <-a.eventLogs
		bytes, err := json.Marshal(entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal event log to json: %s", err.Error())
			continue
		}

		subject := fmt.Sprintf("agentint.%s.events.%s", *a.md.VmID, entry.Type())
		err = a.nc.Publish(subject, bytes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to publish event: %s", err.Error())
			continue
		}

		a.nc.Flush()
	}
}

// This is run inside a goroutine to pull log entries off the channel and publish to the
// node host via internal NATS
func (a *Agent) dispatchLogs() {
	for !a.shuttingDown() {
		entry := <-a.agentLogs
		bytes, err := json.Marshal(entry)
		if err != nil {
			continue
		}

		subject := fmt.Sprintf("agentint.%s.logs", *a.md.VmID)
		err = a.nc.Publish(subject, bytes)
		if err != nil {
			continue
		}

		a.nc.Flush()
	}
}

// Pull a deploy request off the wire, get the payload from the shared
// bucket, write it to tmp, initialize the execution provider per the
// request, and then validate and deploy a workload
func (a *Agent) handleDeploy(m *nats.Msg) {
	var request agentapi.DeployRequest
	err := json.Unmarshal(m.Data, &request)
	if err != nil {
		msg := fmt.Sprintf("Failed to unmarshal deploy request: %s", err)
		a.LogError(msg)
		_ = a.workAck(m, false, msg)
		return
	}

	err = request.Validate()
	if err != nil {
		_ = a.workAck(m, false, fmt.Sprintf("%v", err)) // FIXME-- this message can be formatted prettier
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
	a.provider = provider

	shouldValidate := true
	if !a.sandboxed && request.WorkloadType == models.NexExecutionProviderNative {
		shouldValidate = false
	}

	if shouldValidate {
		err = a.provider.Validate()
		if err != nil {
			msg := fmt.Sprintf("Failed to validate workload: %s", err)
			a.LogError(msg)
			_ = a.workAck(m, false, msg)
			return
		}
	}

	err = a.provider.Deploy()
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to deploy workload: %s", err))
	} else {
		_ = a.workAck(m, true, "Workload deployed")
	}
}

func (a *Agent) handleUndeploy(m *nats.Msg) {
	if a.provider == nil {
		a.LogDebug("Received undeploy workload request on agent without deployed workload")
		_ = m.Respond([]byte{})
		return
	}

	err := a.provider.Undeploy()
	if err != nil {
		// don't return an error here so worst-case scenario is an ungraceful shutdown,
		// not a failure
		a.LogError(fmt.Sprintf("Failed to undeploy workload: %s", err))
	}

	_ = m.Respond([]byte{})
}

func (a *Agent) handlePing(m *nats.Msg) {
	_ = m.Respond([]byte("OK"))
}

func (a *Agent) init() error {
	a.installSignalHandlers()

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	err := a.requestHandshake()
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to handshake with node: %s", err))
		return err
	}

	subject := fmt.Sprintf("agentint.%s.deploy", *a.md.VmID)
	_, err = a.nc.Subscribe(subject, a.handleDeploy)
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to subscribe to agent deploy subject: %s", err))
		return err
	}

	udsubject := fmt.Sprintf("agentint.%s.undeploy", *a.md.VmID)
	_, err = a.nc.Subscribe(udsubject, a.handleUndeploy)
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to subscribe to agent undeploy subject: %s", err))
		return err
	}

	pingSubject := fmt.Sprintf("agentint.%s.ping", *a.md.VmID)
	_, err = a.nc.Subscribe(pingSubject, a.handlePing)
	if err != nil {
		a.LogError(fmt.Sprintf("failed to subscribe to ping subject: %s", err))
	}

	go a.dispatchEvents()
	go a.dispatchLogs()

	return nil
}

func (a *Agent) installSignalHandlers() {
	signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	resetSIGUSR()
	a.sigs = make(chan os.Signal, 1)
	signal.Notify(a.sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
}

// newExecutionProviderParams initializes new execution provider params
// for the given work request and starts a goroutine listening
func (a *Agent) newExecutionProviderParams(req *agentapi.DeployRequest, tmpFile string) (*agentapi.ExecutionProviderParams, error) {
	if a.md.VmID == nil {
		return nil, errors.New("vm id is required to initialize execution provider params")
	}

	if req.WorkloadName == nil {
		return nil, errors.New("workload name is required to initialize execution provider params")
	}

	params := &agentapi.ExecutionProviderParams{
		DeployRequest: *req,
		Stderr:        &logEmitter{stderr: true, name: *req.WorkloadName, logs: a.agentLogs},
		Stdout:        &logEmitter{stderr: false, name: *req.WorkloadName, logs: a.agentLogs},
		TmpFilename:   &tmpFile,
		VmID:          *a.md.VmID,

		Fail: make(chan bool),
		Run:  make(chan bool),
		Exit: make(chan int),

		NATSConn:        a.nc,
		TriggerSubjects: req.TriggerSubjects,
	}

	go func() {
		sleepMillis := agentapi.DefaultRunloopSleepTimeoutMillis

		for {
			select {
			case <-params.Fail:
				msg := fmt.Sprintf("Failed to start workload: %s; vm: %s", *params.WorkloadName, params.VmID)
				a.PublishWorkloadExited(params.VmID, *params.WorkloadName, msg, true, -1)
				return

			case <-params.Run:
				a.PublishWorkloadDeployed(params.VmID, *params.WorkloadName, params.TotalBytes)
				sleepMillis = workloadExecutionSleepTimeoutMillis

			case exit := <-params.Exit:
				msg := fmt.Sprintf("Exited workload: %s; vm: %s; status: %d", *params.WorkloadName, params.VmID, exit)
				a.PublishWorkloadExited(params.VmID, *params.WorkloadName, msg, exit != 0, exit)
				return
			default:
				// no-op
			}

			time.Sleep(time.Millisecond * time.Duration(sleepMillis))
		}
	}()

	return params, nil
}

func (a *Agent) shutdown() {
	if atomic.AddUint32(&a.closing, 1) == 1 {
		if a.provider != nil {
			err := a.provider.Undeploy()
			if err != nil {
				fmt.Printf("failed to undeploy workload: %s", err)
			}
		}

		_ = a.nc.Drain()
		for !a.nc.IsClosed() {
			time.Sleep(time.Millisecond * 25)
		}

		HaltVM(nil)
	}
}

func (a *Agent) shuttingDown() bool {
	return (atomic.LoadUint32(&a.closing) > 0)
}

func (a *Agent) submitLog(msg string, lvl agentapi.LogLevel) {
	a.agentLogs <- &agentapi.LogEntry{
		Source: NexEventSourceNexAgent,
		Level:  lvl,
		Text:   msg,
	}
}

// workAck ACKs the provided NATS message by responding with the
// accepted status of the attempted work request and associated message
func (a *Agent) workAck(m *nats.Msg, accepted bool, msg string) error {
	ack := agentapi.DeployResponse{
		Accepted: accepted,
		Message:  models.StringOrNil(msg),
	}

	bytes, err := json.Marshal(&ack)
	if err != nil {
		return err
	}

	err = m.Respond(bytes)
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to acknowledge workload deployment: %s", err))
		return err
	}

	return nil
}

func isSandboxed() bool {
	return !strings.EqualFold(strings.ToLower(os.Getenv(nexEnvSandbox)), "false")
}
