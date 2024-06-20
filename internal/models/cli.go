package models

import (
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats.go"
	controlapi "github.com/synadia-io/nex/control-api"
)

type UiOptions struct {
	Port int
}

type DevRunOptions struct {
	Filename string
	// Stop a workload with the same name on a target
	AutoStop bool
	// Max bytes override for when we create the NEXCLIFILES bucket
	DevBucketMaxBytes uint
}

// Options configure the CLI
type Options struct {
	Servers string
	// Creds is nats credentials to authenticate with
	Creds string
	// TlsCert is the TLS Public Certificate
	TlsCert string
	// TlsKey is the TLS Private Key
	TlsKey string
	// TlsCA is the certificate authority to verify the connection with
	TlsCA string
	// Timeout is how long to wait for operations
	Timeout time.Duration
	// ConnectionName is the name to use for the underlying NATS connection
	ConnectionName string
	// Username is the username or token to connect with
	Username string
	// Password is the password to connect with
	Password string
	// Nkey is the file holding a nkey to connect with
	Nkey string
	// JsDomain is the domain to use for JetStream
	JsDomain string
	// Trace enables verbose debug logging
	Trace bool
	// SocksProxy is a SOCKS5 proxy to use for NATS connections
	SocksProxy string
	// TlsFirst configures theTLSHandshakeFirst behavior in nats.go
	TlsFirst bool
	// Namespace for scoping workload requests
	Namespace string
	// Type of logger
	Logger []string
	// LogLevel is the log level to use
	LogLevel string
	// LogJSON enables JSON logging
	LogTimeFormat string
	// Timeformat for logs.  Must satisfy time.Time format criteria
	LogsColorized bool
	// Outputs logs with color
	LogJSON bool
	// Name or path to a configuration context
	ConfigurationContext string
}

type RunOptions struct {
	Argv              string
	TargetNode        string
	WorkloadUrl       *url.URL
	Name              string
	WorkloadType      controlapi.NexWorkload
	Description       string
	PublisherXkeyFile string
	ClaimsIssuerFile  string
	Env               map[string]string
	Essential         bool
	DevMode           bool
	TriggerSubjects   []string

	HsUrl      string
	HsUserJwt  string
	HsUserSeed string
}

type StopOptions struct {
	TargetNode       string
	WorkloadName     string
	WorkloadId       string
	ClaimsIssuerFile string
}

type WatchOptions struct {
	NodeId       string
	WorkloadId   string
	WorkloadName string
	LogLevel     string
}

type RootfsOptions struct {
	OutName         string
	BaseImage       string
	BuildScriptPath string
	AgentBinaryPath string
	RootFSSize      int
}

// Node configuration is used to configure the node process as well
// as the virtual machines it produces
type NodeOptions struct {
	ConfigFilepath  string   `json:"-"`
	ForceDepInstall bool     `json:"-"`
	CniNS           []string `json:"-"`

	OtelMetrics         bool   `json:"-"`
	OtelMetricsPort     int    `json:"-"`
	OtelMetricsExporter string `json:"-"`
	OtelTraces          bool   `json:"-"`
	OtelTracesExporter  string `json:"-"`

	PreflightInit    string `json:"-"`
	PreflightVerify  bool   `json:"-"`
	PreflightVerbose bool   `json:"-"`
	PreflightCheck   bool   `json:"-"`

	ListFull  bool   `json:"-"`
	NexusName string `json:"-"`

	Errors []error `json:"errors,omitempty"`
}

func (c *NodeOptions) Validate() bool {
	c.Errors = make([]error, 0)

	// TODO-- add validation

	return len(c.Errors) == 0
}

func GenerateConnectionFromOpts(opts *Options, logger *slog.Logger) (*nats.Conn, error) {
	if opts.ConfigurationContext != "" {
		if !natscontext.IsKnown(opts.ConfigurationContext) {
			logger.Error("Unknown nats context provided", slog.String("context", opts.ConfigurationContext))
			return nil, fmt.Errorf("unknown context provided")
		}

		p, _ := natscontext.ContextPath(opts.ConfigurationContext)
		logger.Debug("Using nats context for connection details", slog.String("path", p))

		conn, err := natscontext.Connect(opts.ConfigurationContext, nats.Name(opts.ConnectionName))
		logger.Info("Connected to NATS server", slog.String("server", conn.ConnectedUrlRedacted()), slog.String("nats_context", opts.ConfigurationContext))

		return conn, err
	}

	natsOpts := []nats.Option{
		nats.Name(opts.ConnectionName),
	}

	if opts.Creds != "" {
		natsOpts = append(natsOpts, nats.UserCredentials(opts.Creds))
	}

	if opts.Nkey != "" {
		natsOpts = append(natsOpts, nats.Nkey(opts.Nkey, nats.DefaultOptions.SignatureCB))
	}

	if opts.TlsCert != "" && opts.TlsKey != "" {
		natsOpts = append(natsOpts, nats.ClientCert(opts.TlsCert, opts.TlsKey))
	}

	if opts.TlsCA != "" {
		natsOpts = append(natsOpts, nats.RootCAs(opts.TlsCA))
	}

	if opts.TlsFirst {
		natsOpts = append(natsOpts, nats.TLSHandshakeFirst())
	}

	if opts.Username != "" && opts.Password == "" {
		natsOpts = append(natsOpts, nats.Token(opts.Username))
	} else {
		natsOpts = append(natsOpts, nats.UserInfo(opts.Username, opts.Password))
	}

	conn, err := nats.Connect(opts.Servers, natsOpts...)
	logger.Debug("Connected to NATS server",
		slog.String("server", conn.ConnectedUrlRedacted()),
		slog.String("name", opts.ConnectionName),
		slog.String("creds", opts.Creds),
		slog.String("nkey", opts.Nkey),
		slog.String("tls_cert", opts.TlsCert),
		slog.String("tls_key", opts.TlsKey),
		slog.String("tls_ca", opts.TlsCA),
		slog.Bool("tls_first", opts.TlsFirst),
		slog.String("username", opts.Username),
	)

	return conn, err
}
