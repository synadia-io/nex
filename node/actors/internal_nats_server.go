package actors

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"text/template"
	"time"

	"disorder.dev/shandler"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
)

const (
	defaultInternalNatsConfigFile             = "internalconf*"
	defaultInternalNatsConnectionDrainTimeout = time.Millisecond * 5000
	workloadCacheBucketName                   = "NEXCACHE"
	InternalNatsServerActorName               = "internal_nats"
)

type InternalNatsServer struct {
	server        *server.Server
	nodeOptions   models.NodeOptions
	hostUser      nkeys.KeyPair
	creds         []AgentCredential
	serverOptions *server.Options
	logger        *slog.Logger

	storeDir string
}

func CreateInternalNatsServer(options models.NodeOptions) *InternalNatsServer {
	ns := &InternalNatsServer{nodeOptions: options, logger: options.Logger}

	hostUser, err := nkeys.CreateUser()
	if err != nil {
		options.Logger.Error("Failed to create host user", slog.Any("error", err))
		return nil
	}
	ns.hostUser = hostUser

	creds, err := ns.buildAgentCredentials()
	if err != nil {
		options.Logger.Error("Failed to build agent credentials", slog.Any("error", err))
		return nil
	}
	ns.creds = creds

	opts, err := ns.generateConfig()
	if err != nil {
		options.Logger.Error("Failed to generate NATS server config", slog.Any("error", err))
		return nil
	}
	ns.serverOptions = opts

	ns.storeDir, err = os.MkdirTemp(os.TempDir(), "pnats_*")
	if err != nil {
		options.Logger.Error("Failed to create store temp directory", slog.Any("error", err))
		return nil
	}

	return ns
}

func (ns *InternalNatsServer) CredentialsMap() map[string]AgentCredential {
	out := make(map[string]AgentCredential)
	for _, cred := range ns.creds {
		out[cred.workloadType] = cred
	}

	return out
}

func (ns *InternalNatsServer) PreStart(ctx context.Context) error {

	err := ns.startNatsServer(ns.serverOptions)
	if err != nil {
		return nil
	}

	return nil
}

func (s *InternalNatsServer) PostStop(ctx context.Context) error {
	return nil
}

func (s *InternalNatsServer) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		s.logger.Info("Internal NATS server actor is running", slog.String("name", ctx.Self().Name()))
	default:
		ctx.Unhandled()
	}
}

func (ns *InternalNatsServer) buildAgentCredentials() ([]AgentCredential, error) {
	creds := make([]AgentCredential, len(ns.nodeOptions.AgentOptions))
	for i, w := range ns.nodeOptions.AgentOptions {
		kp, _ := nkeys.CreateUser()
		creds[i] = AgentCredential{
			workloadType: w.Name,
			nkey:         kp,
		}
	}
	return creds, nil
}

type AgentCredential struct {
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

func (ns *InternalNatsServer) startNatsServer(opts *server.Options) error {
	ns.logger.Debug("Starting internal NATS server")
	var err error
	ns.server, err = server.NewServer(opts)
	if err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
		return err
	}

	// FIXME-- read from config
	// if debug || trace {
	// 	s.ConfigureLogger()
	// }

	if err := server.Run(ns.server); err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
		return err
	}

	return nil
}

func (ns *InternalNatsServer) generateConfig() (*server.Options, error) {
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
		ns.logger.Error("Failed to generate internal nats server config file", slog.Any("error", err))
		return nil, err
	}

	opts := &server.Options{
		JetStream: true,
		StoreDir:  ns.storeDir,
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
		ns.logger.Error("Failed to write internal nats server config file", slog.Any("error", err))
		return nil, err
	}

	err = opts.ProcessConfigFile(f.Name())
	if err != nil {
		ns.logger.Error("Failed to process configuration file", slog.Any("error", err))
		return nil, err
	}

	return opts, nil
}

func (ns *InternalNatsServer) generateTemplate(config *configTemplateData) ([]byte, error) {
	var wr bytes.Buffer

	t := template.Must(template.New("natsconfig").Parse(configTemplate))
	err := t.Execute(&wr, config)
	if err != nil {
		return nil, err
	}

	ns.logger.Log(context.TODO(), shandler.LevelTrace, "generated NATS config", slog.String("config", wr.String()))
	return wr.Bytes(), nil
}

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
