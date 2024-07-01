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
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/agent/providers"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/synadia-io/nex/internal/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const (
	defaultAgentHandshakeAttempts       = 5
	defaultAgentHandshakeTimeoutMillis  = 500
	runloopSleepInterval                = 250 * time.Millisecond
	runloopTickInterval                 = 2500 * time.Millisecond
	workloadExecutionSleepTimeoutMillis = 100
	workloadCacheFileKey                = "workload"
	workloadLocationSchemeFile          = "file"
	workloadLocationSchemeNATS          = "nats"
)

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
	subz     []*nats.Subscription

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
		fmt.Fprintf(os.Stderr, "failed to get machine metadata: %s\n", err)
		return nil, fmt.Errorf("failed to get machine metadata: %s", err)
	}

	if !metadata.Validate() {
		fmt.Fprintf(os.Stderr, "invalid metadata: %v\n", metadata.Errors)
		return nil, fmt.Errorf("invalid metadata: %v", metadata.Errors)
	}

	return &Agent{
		agentLogs: make(chan *agentapi.LogEntry, 64),
		eventLogs: make(chan *cloudevents.Event, 64),
		// sandbox defaults to true, only way to override that is with an explicit 'false'
		cancelF:   cancelF,
		ctx:       ctx,
		sandboxed: isSandboxed(),
		md:        metadata,
		started:   time.Now().UTC(),
		subz:      make([]*nats.Subscription, 0),
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

	attempts := 0
	for attempts < defaultAgentHandshakeAttempts-1 && !a.shuttingDown() {
		attempts++

		resp, err := a.nc.Request(fmt.Sprintf("hostint.%s.handshake", *a.md.VmID), raw, time.Millisecond*defaultAgentHandshakeTimeoutMillis)
		if err != nil {
			a.LogError(fmt.Sprintf("Agent failed to request initial sync message: %s, attempt %d", err, attempts+1))
			time.Sleep(time.Millisecond * 25)
			continue
		}

		var handshakeResponse *agentapi.HandshakeResponse
		err = json.Unmarshal(resp.Data, &handshakeResponse)
		if err != nil {
			a.LogError(fmt.Sprintf("Failed to parse handshake response: %s", err))
			time.Sleep(time.Millisecond * 25)
			continue
		}

		a.LogInfo(fmt.Sprintf("Agent is up after %d attempt(s)", attempts))
		return nil
	}

	return errors.New("Failed to obtain handshake from host")
}

func (a *Agent) Version() string {
	return VERSION
}

// resolveExecutableArtifact uses the underlying agent configuration and
// location specified in the workload deploy request to prepare the workload
// artifact for execution by the agent.
//
// in the case of the workload artifact being deployed from a nats:// location
// this method fetches the workload artifact from the object store bucket,
// writes it to a temporary file and makes it executable.
//
// in the case of the workload artifact being deployed from a file:// location,
// this method verifies that the specified file exists on the agent rootfs and
// is executable.
func (a *Agent) resolveExecutableArtifact(req *agentapi.DeployRequest) (*string, error) {
	var file *string

	switch strings.ToLower(req.Location.Scheme) {
	case workloadLocationSchemeFile:
		if !isSandboxed() {
			return nil, fmt.Errorf("attempted to execute agent-local workload artifact %s outside of sandbox ", req.Location.String())
		}

		path, _ := strings.CutPrefix(req.Location.String(), fmt.Sprintf("%s://", workloadLocationSchemeFile))

		fi, err := os.Stat(path)
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("attempted to execute agent-local workload artifact %s; file does not exist: %s", path, err.Error())
		}

		if err != nil {
			return nil, fmt.Errorf("failed to stat agent-local workload artifact %s; %s", path, err.Error())
		}

		if fi.IsDir() {
			return nil, fmt.Errorf("failed to execute agent-local workload artifact; %s is a directory", path)
		} else if fi.Mode()&0111 == 0 {
			return nil, fmt.Errorf("failed to execute agent-local workload artifact; %s is not executable", path)
		}

		file = &path
	case workloadLocationSchemeNATS:
		fileName := fmt.Sprintf("workload-%s", *a.md.VmID)
		tempFile := path.Join(os.TempDir(), fileName)

		if strings.EqualFold(runtime.GOOS, "windows") && req.WorkloadType == controlapi.NexWorkloadNative {
			tempFile = fmt.Sprintf("%s.exe", tempFile)
		}

		err := a.cacheBucket.GetFile(workloadCacheFileKey, tempFile)
		if err != nil {
			msg := fmt.Sprintf("Failed to get and write workload artifact to temp dir: %s", err)
			a.LogError(msg)
			return nil, errors.New(msg)
		}

		err = os.Chmod(tempFile, 0777)
		if err != nil {
			msg := fmt.Sprintf("Failed to set workload artifact as executable: %s", err)
			a.LogError(msg)
			return nil, errors.New(msg)
		}

		file = &tempFile
	}

	return file, nil
}

// deleteExecutableArtifact deletes the installed workload executable
// and purges it from the internal object store
func (a *Agent) deleteExecutableArtifact() error {
	fileName := fmt.Sprintf("workload-%s", *a.md.VmID)
	tempFile := path.Join(os.TempDir(), fileName)

	_ = os.Remove(tempFile)
	_ = a.cacheBucket.Delete(*a.md.VmID)

	return nil
}

// Run inside a goroutine to pull event entries and publish them to the node host.
func (a *Agent) dispatchEvents() {
	for !a.shuttingDown() {
		entry := <-a.eventLogs
		bytes, err := entry.MarshalJSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal event log to json: %s", err.Error())
			continue
		}

		subject := fmt.Sprintf("hostint.%s.events.%s", *a.md.VmID, entry.Type())
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

		subject := fmt.Sprintf("hostint.%s.logs", *a.md.VmID)
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

	path, err := a.resolveExecutableArtifact(&request)
	if err != nil {
		_ = a.workAck(m, false, err.Error())
		return
	}

	params, err := a.newExecutionProviderParams(&request, *path)
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
	if !a.sandboxed && request.WorkloadType == controlapi.NexWorkloadNative {
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
	// a.LogDebug(fmt.Sprintf("received ping on subject: %s", m.Subject))
	_ = m.Respond([]byte("OK"))
}

// Agent instances subscribe to the following `agentint.>` subjects,
// which are exported dynamically by each `<agent_id>` account on the
// configured internal NATS connection for consumption by the nex node:
//
// - agentint.<agent_id>.deploy
// - agentint.<agent_id>.undeploy
// - agentint.<agent_id>.ping
func (a *Agent) init() error {
	a.installSignalHandlers()

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	err := a.initNATS()
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to initialize NATS connection: %s", err))
		return err
	}

	err = a.requestHandshake()
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to handshake with node: %s", err))
		return err
	}

	subject := fmt.Sprintf("agentint.%s.deploy", *a.md.VmID)
	sub, err := a.nc.Subscribe(subject, a.handleDeploy)
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to subscribe to agent deploy subject: %s", err))
		return err
	}
	a.subz = append(a.subz, sub)

	udsubject := fmt.Sprintf("agentint.%s.undeploy", *a.md.VmID)
	sub, err = a.nc.Subscribe(udsubject, a.handleUndeploy)
	if err != nil {
		a.LogError(fmt.Sprintf("Failed to subscribe to agent undeploy subject: %s", err))
		return err
	}
	a.subz = append(a.subz, sub)

	pingSubject := fmt.Sprintf("agentint.%s.ping", *a.md.VmID)
	sub, err = a.nc.Subscribe(pingSubject, a.handlePing)
	if err != nil {
		a.LogError(fmt.Sprintf("failed to subscribe to ping subject: %s", err))
	}
	a.subz = append(a.subz, sub)

	go a.dispatchEvents()
	go a.dispatchLogs()

	return nil
}

func (a *Agent) initNATS() error {
	url := fmt.Sprintf("nats://%s:%d", *a.md.NodeNatsHost, *a.md.NodeNatsPort)
	pair, err := nkeys.FromSeed([]byte(*a.md.NodeNatsNkeySeed))
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid nkey seed: %v\n", *a.md.NodeNatsNkeySeed)
		return fmt.Errorf("invalid nkey seed: %v", *a.md.NodeNatsNkeySeed)
	}

	pk, _ := pair.PublicKey()
	for attempt := 0; attempt < 3; attempt++ {
		a.nc, err = nats.Connect(url, nats.Nkey(pk, func(b []byte) ([]byte, error) {
			fmt.Fprintf(os.Stdout, "Attempting to authenticate to internal NATS as %s", pk)
			return pair.Sign(b)
		}))
		if err != nil {
			time.Sleep(100 * time.Millisecond)
		} else {
			break
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to internal NATS: %s", err)
		return err
	}

	fmt.Printf("Connected to internal NATS: %s\n", url)

	js, err := a.nc.JetStream()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get JetStream context from internal NATS: %s", err)
		return err
	}

	a.cacheBucket, err = js.ObjectStore(agentapi.WorkloadCacheBucket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get reference to internal object store: %s", err)
		return err
	}

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
		PluginPath:      a.md.PluginPath,
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
		_ = a.deleteExecutableArtifact()

		for _, sub := range a.subz {
			_ = sub.Drain()
		}

		if a.nc != nil {
			_ = a.nc.Drain()
			for !a.nc.IsClosed() {
				time.Sleep(time.Millisecond * 25)
			}
		}

		if a.provider != nil {
			err := a.provider.Undeploy()
			if err != nil {
				fmt.Printf("failed to undeploy workload: %s\n", err)
			}
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
