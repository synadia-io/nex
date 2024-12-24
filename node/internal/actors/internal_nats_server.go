package actors

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"disorder.dev/shandler"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
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
	devMode       bool
	storeDir      string
}

func (ns *InternalNatsServer) GetHostKeyPair() nkeys.KeyPair {
	return ns.hostUser
}

func (ns *InternalNatsServer) GetServerURL() string {
	return ns.server.ClientURL()
}

func CreateInternalNatsServer(serverPubKey string, options models.NodeOptions) (*InternalNatsServer, error) {
	ns := &InternalNatsServer{nodeOptions: options, logger: options.Logger}

	ns.devMode = options.DevMode
	if ns.devMode {
		ns.logger.Warn("DO NOT USE IN PRODUCTION: Running in dev mode, using default credentials")
	}

	hostUser, err := nkeys.CreateUser()
	if err != nil {
		options.Logger.Error("Failed to create host user", slog.Any("error", err))
		return nil, err
	}
	ns.hostUser = hostUser

	creds, err := ns.buildAgentCredentials()
	if err != nil {
		options.Logger.Error("Failed to build agent credentials", slog.Any("error", err))
		return nil, err
	}
	ns.creds = creds

	err = os.Mkdir(filepath.Join(os.TempDir(), "inex-"+serverPubKey), 0700)
	if err != nil && !os.IsExist(err) {
		options.Logger.Error("Failed to create store temp directory", slog.Any("error", err))
		return nil, err
	}

	ns.storeDir = filepath.Join(os.TempDir(), "inex-"+serverPubKey)

	opts, err := ns.generateConfig()
	if err != nil {
		options.Logger.Error("Failed to generate NATS server config", slog.Any("error", err))
		return nil, err
	}

	ns.serverOptions = opts
	return ns, nil
}

func (ns *InternalNatsServer) CredentialsMap() map[string]AgentCredential {
	out := make(map[string]AgentCredential)
	for _, cred := range ns.creds {
		out[cred.workloadType] = cred
	}

	return out
}

func (ns *InternalNatsServer) PreStart(ctx context.Context) error {
	return nil
}

func (s *InternalNatsServer) PostStop(ctx context.Context) error {
	return nil
}

func (s *InternalNatsServer) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		err := s.startNatsServer(s.serverOptions)
		if err != nil {
			ctx.Err(err)
		}
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
	DevMode           bool
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
	var err error
	ns.server, err = server.NewServer(opts)
	if err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
		return err
	}
	// ns.server.ConfigureLogger()

	if err := server.Run(ns.server); err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
		return err
	}

	hostPub, err := ns.hostUser.PublicKey()
	if err != nil {
		return err
	}

	nc, err := nats.Connect(ns.server.ClientURL(), nats.Nkey(hostPub, func(nonce []byte) ([]byte, error) { return ns.hostUser.Sign(nonce) }))
	if err != nil {
		return err
	}
	defer nc.Close()

	jsCtx, err := jetstream.New(nc)
	if err != nil {
		return err
	}

	_, err = jsCtx.CreateOrUpdateKeyValue(context.TODO(), jetstream.KeyValueConfig{
		Bucket:       models.RunRequestKVBucket,
		MaxBytes:     50000000, // 50MB
		MaxValueSize: 10000,    // 10KB
	})

	if err != nil {
		return err
	}

	ns.logger.Debug("Internal NATS server running", slog.String("url", ns.server.ClientURL()))
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
		DevMode:           ns.devMode,
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
		NoSigs:    true,
	}

	opts.Debug = true
	opts.Trace = true

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
      {{ if .DevMode }} { user: admin, password: password } {{ end }}
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
