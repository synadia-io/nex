package actors

import (
	"bytes"
	"errors"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
)

const (
	defaultInternalNatsConfigFile             = "internalconf*"
	defaultInternalNatsConnectionDrainTimeout = time.Millisecond * 5000
	defaultInternalNatsStoreDir               = "pnats"
	workloadCacheBucketName                   = "NEXCACHE"
)

func createInternalNatsServer() gen.ProcessBehavior {
	return &internalNatsServer{}
}

type internalNatsServer struct {
	act.Actor
	tokens        map[gen.Atom]gen.Ref
	haveConsumers bool

	creds       []agentCredential
	hostUser    nkeys.KeyPair
	nodeOptions models.NodeOptions
}

type internalNatsServerParams struct {
	nodeOptions models.NodeOptions
}

func (p *internalNatsServerParams) Validate() error {
	var err error

	// insert validations
	// validate options much?

	return err
}

type agentCredential struct {
	workloadType string
	nkey         nkeys.KeyPair
}

type configTemplateData struct {
	Credentials       map[string]*credentials
	Connections       map[string]*nats.Conn
	NexHostUserPublic string
	NexHostUserSeed   string
}

type credentials struct {
	WorkloadType string
	NkeySeed     string
	NkeyPublic   string
}

func (ns *internalNatsServer) Init(args ...any) error {
	ns.tokens = make(map[gen.Atom]gen.Ref)

	if len(args) != 1 {
		err := errors.New("internal NATS server params are required")
		ns.Log().Error("Failed to start internal NATS server", slog.String("error", err.Error()))
		return err
	}

	if _, ok := args[0].(internalNatsServerParams); !ok {
		err := errors.New("args[0] must be valid internal NATS server params")
		ns.Log().Error("Failed to start internal NATS server", slog.String("error", err.Error()))
		return err
	}

	params := args[0].(internalNatsServerParams)
	err := params.Validate()
	if err != nil {
		ns.Log().Error("Failed to start internal NATS server", slog.String("error", err.Error()))
		return err
	}

	ns.nodeOptions = params.nodeOptions

	hostUser, err := nkeys.CreateUser()
	if err != nil {
		return err
	}
	ns.hostUser = hostUser

	creds, err := ns.buildAgentCredentials()
	if err != nil {
		return err
	}

	ns.creds = creds

	opts, err := ns.generateConfig()
	if err != nil {
		return err
	}

	err = ns.startNatsServer(opts)
	if err != nil {
		return err
	}

	err = ns.Send(ns.PID(), PostInit)
	if err != nil {
		return err
	}

	ns.Log().Info("Internal NATS server started")

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

func (ns *internalNatsServer) startNatsServer(opts *server.Options) error {
	ns.Log().Debug("Starting internal NATS server")

	s, err := server.NewServer(opts)
	if err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
		return err
	}

	// FIXME-- read from config
	// if debug || trace {
	// 	s.ConfigureLogger()
	// }

	if err := server.Run(s); err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
		return err
	}

	return nil
}

func (ns *internalNatsServer) generateConfig() (*server.Options, error) {
	hostPub, err := ns.hostUser.PublicKey()
	if err != nil {
		return nil, err
	}

	hostSeed, err := ns.hostUser.Seed()
	if err != nil {
		return nil, err
	}

	data := &configTemplateData{
		Credentials:       make(map[string]*credentials),
		Connections:       make(map[string]*nats.Conn),
		NexHostUserPublic: hostPub,
		NexHostUserSeed:   string(hostSeed),
	}

	for _, cred := range ns.creds {
		seed, err := cred.nkey.Seed()
		if err != nil {
			return nil, err
		}

		pubkey, err := cred.nkey.PublicKey()
		if err != nil {
			return nil, err
		}

		data.Credentials[cred.workloadType] = &credentials{
			WorkloadType: cred.workloadType,
			NkeySeed:     string(seed),
			NkeyPublic:   pubkey,
		}
	}

	bytes, err := ns.generateTemplate(data)
	if err != nil {
		ns.Log().Error("Failed to generate internal nats server config file", slog.Any("error", err))
		return nil, err
	}

	opts := &server.Options{
		JetStream: true,
		StoreDir:  filepath.Join(os.TempDir(), defaultInternalNatsStoreDir), // FIXME-- use unique temp location
		Port:      -1,
		// Debug:     debug, FIXME-- make configurable
		// Trace:     trace,
	}

	f, err := os.CreateTemp(os.TempDir(), defaultInternalNatsConfigFile)
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name()) // clean up

	if _, err := f.Write(bytes); err != nil {
		ns.Log().Error("Failed to write internal nats server config file", slog.Any("error", err))
		return nil, err
	}

	err = opts.ProcessConfigFile(f.Name())
	if err != nil {
		ns.Log().Error("Failed to process configuration file", slog.Any("error", err))
		return nil, err
	}

	return opts, nil
}

func (ns *internalNatsServer) generateTemplate(config *configTemplateData) ([]byte, error) {
	var wr bytes.Buffer

	t := template.Must(template.New("natsconfig").Parse(configTemplate))
	err := t.Execute(&wr, config)
	if err != nil {
		return nil, err
	}

	// -8 is equivalent to TRACE-ish level
	// FIXME... .Log() does not exist
	// ns.Log().Log(context.Background(), -8, "generated NATS config", ns.Log().String("config", wr.String()))
	return wr.Bytes(), nil
}

// func getPort(clientUrl string) int {
// 	u, err := url.Parse(clientUrl)
// 	if err != nil {
// 		return -1
// 	}
// 	res, err := strconv.Atoi(u.Port())
// 	if err != nil {
// 		return -1
// 	}
// 	return res
// }

/*
 * In the below template, the nexhost (the account used by the host) will
 * be able to import the following:
 * *.agentevt.> - agent events streamed from workloads
 * hostint.> - where the first token after agentint is the account ID from the workload
 */

const (
	configTemplate = `
jetstream: true
accounts: {
	nexhost: {
		jetstream: true
		users: [
			{nkey: "{{ .NexHostUserPublic }}"}
		]
		exports: [
			{
				service: hostint.>
			}
		],
		imports: [
			{{ range .Credentials }}
			{
				service: {subject: "agentint.{{ .WorkloadType }}.>", account: "{{ .WorkloadType }}"}
			},
			{
				stream: {subject: agentevt.>, account: "{{ .WorkloadType }}"}, prefix: "{{ .WorkloadType }}"
			},
			{{ end }}
		]
	},
	{{ range .Credentials }}
	"{{ .WorkloadType }}": {
		jetstream: true
		users: [
			{nkey: "{{ .NkeyPublic }}"}
		]
		exports: [
			{
				service: "agentint.{{ .WorkloadType }}.>", accounts: [nexhost]
			}
			{
				stream: agentevt.>, accounts: [nexhost]
			}
		]
		imports: [
			{
				service: {account: nexhost, subject: "hostint.{{ .WorkloadType }}.>"}, to: "hostint.>"
			}
		]

	},
	{{ end }}
}
no_sys_acc: true
debug: true
trace: false
`
)
