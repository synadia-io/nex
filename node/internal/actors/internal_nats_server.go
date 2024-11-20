package actors

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"text/template"
	"time"

	"disorder.dev/shandler"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
)

const (
	defaultInternalNatsConfigFile             = "internalconf*"
	defaultInternalNatsConnectionDrainTimeout = time.Millisecond * 5000
	workloadCacheBucketName                   = "NEXCACHE"
	InternalNatsServerActorName               = "internal_nats"

	RegisterAgentResponseType = "io.nats.nex.v2.register_response"
)

type InternalNatsServer struct {
	server      *server.Server
	nodeOptions models.NodeOptions
	hostUser    nkeys.KeyPair
	creds       []AgentCredential

	internalNatsUrl string
	conn            *nats.Conn

	serverOptions *server.Options
	logger        *slog.Logger

	self     *goakt.PID
	storeDir string
}

func CreateInternalNatsServer(options models.NodeOptions, logger *slog.Logger) *InternalNatsServer {
	ns := &InternalNatsServer{nodeOptions: options, logger: logger}

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

	ns.storeDir, err = os.MkdirTemp(os.TempDir(), "pnats_*")
	if err != nil {
		options.Logger.Error("Failed to create store temp directory", slog.Any("error", err))
		return nil
	}

	opts, err := ns.generateConfig()
	if err != nil {
		options.Logger.Error("Failed to generate NATS server config", slog.Any("error", err))
		return nil
	}
	ns.serverOptions = opts

	err = ns.startNatsServer(opts)
	if err != nil {
		options.Logger.Error("Failed to start server", slog.Any("error", err))
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

func (ns *InternalNatsServer) ServerUrl() string {
	return ns.internalNatsUrl
}

func (ns *InternalNatsServer) HostUserKeypair() nkeys.KeyPair {
	return ns.hostUser
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
		s.self = ctx.Self()
		s.logger.Debug("Internal NATS server actor is running", slog.String("name", ctx.Self().Name()))
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
	var err error
	ns.server, err = server.NewServer(opts)
	if err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
		return err
	}
	// NOTE: uncomment this if you need to troubleshoot internal NATS comms
	// ns.server.ConfigureLogger()

	if err := server.Run(ns.server); err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
		return err
	}

	clientUrl := ns.server.ClientURL()
	ns.internalNatsUrl = clientUrl

	ns.logger.Debug("Starting internal NATS server", slog.String("client_url", clientUrl))

	pk, err := ns.hostUser.PublicKey()
	if err != nil {
		ns.logger.Error("Failed to get public key for host user")
		return err
	}
	nkeyOpt := nats.Nkey(pk, func(b []byte) ([]byte, error) {
		return ns.hostUser.Sign(b)
	})

	internalClient, err := nats.Connect(clientUrl, nkeyOpt, nats.Name("Nex Node Host User"))
	if err != nil {
		ns.logger.Error("Failed to connect to internal NATS server",
			slog.String("client_url", clientUrl),
			slog.Any("error", err))
		return err
	}

	ns.conn = internalClient

	_, err = ns.conn.Subscribe("host.*.register", ns.handleAgentRegistration)
	if err != nil {
		ns.logger.Error("Failed to subscribe to registration subject",
			slog.Any("error", err))
		return err
	}

	ns.logger.Debug("Connected to internal NATS server as host user", slog.String("client_url", clientUrl))
	return nil
}

func (ns *InternalNatsServer) handleAgentRegistration(msg *nats.Msg) {
	ns.logger.Debug("Received request to register agent", slog.String("subject", msg.Subject))
	tokens := strings.Split(msg.Subject, ".")
	if len(tokens) != 3 {
		ns.logger.Error("Somehow got incorrect number of tokens", slog.String("subject", msg.Subject))
		models.RespondEnvelope(msg, RegisterAgentResponseType, 400, []byte{}, "bad subject")
		return
	}
	ctx := context.Background()
	_, targetAgent, err := ns.self.ActorSystem().ActorOf(ctx, tokens[1])
	if err != nil {
		ns.logger.Error("Got an agent registration from an agent that's not running", slog.String("agent", tokens[1]))
		models.RespondEnvelope(msg, RegisterAgentResponseType, 404, []byte{}, "no such workload type")
		return
	}

	// The direct agent actor communicates with the agent as the host user
	// but needs to pass the agent's internal connection creds to the agent
	// binary
	creds := ns.CredentialsMap()[tokens[1]]
	pubkey, _ := creds.nkey.PublicKey()
	seed, _ := creds.nkey.Seed()

	hspubkey, _ := ns.hostUser.PublicKey()
	hsseed, _ := ns.hostUser.Seed()

	outMsg := &actorproto.AgentRegistered{
		WorkloadType:       tokens[1],
		AgentbinPublicNkey: pubkey,
		AgentbinNkeySeed:   string(seed),
		InternalNatsUrl:    ns.internalNatsUrl,
		InternalNkey:       hspubkey,
		InternalNkeySeed:   string(hsseed),
	}
	err = ns.self.Tell(ctx, targetAgent, outMsg)
	if err != nil {
		ns.logger.Warn("Failed to send 'agent registered' message to agent", slog.String("agent", tokens[1]))
	}
	models.RespondEnvelope(msg, RegisterAgentResponseType, 200, []byte{}, "")
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
		NoSigs:    true,
		//Debug:     true,
		//Trace: true,
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
	err = f.Close()
	if err != nil {
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
				service: host.>
			}
		],
		imports: [
			{{ range .Credentials }}
			{
				service: {subject: "agent.{{ .WorkloadType }}.>", account: "{{ .WorkloadType }}"}
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
				service: "agent.{{ .WorkloadType }}.>", accounts: [nexhost]
			}
			{
				stream: agentevt.>, accounts: [nexhost]
			}
		]
		imports: [
			{
				service: {account: nexhost, subject: "host.{{ .WorkloadType }}.>"}, to: "host.>"
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
