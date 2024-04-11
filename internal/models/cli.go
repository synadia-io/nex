package models

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats.go"
)

type UiOptions struct {
	Port int
}

type DevRunOptions struct {
	Filename string
	// Stop a workload with the same name on a target
	AutoStop bool
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
	// Trace enables verbose debug logging
	Trace bool
	// SocksProxy is a SOCKS5 proxy to use for NATS connections
	SocksProxy string
	// TlsFirst configures theTLSHandshakeFirst behavior in nats.go
	TlsFirst bool
	// Namespace for scoping workload requests
	Namespace string
	// LogLevel is the log level to use
	LogLevel string
	// LogJSON enables JSON logging
	LogJSON bool
	// Name or path to a configuration context
	ConfigurationContext string
	// Effective configuration
	Configuration *natscontext.Context
	// Indicates whether contexts should not be used
	SkipContexts bool
}

type RunOptions struct {
	Argv              string
	TargetNode        string
	WorkloadUrl       *url.URL
	Name              string
	WorkloadType      string
	Description       string
	PublisherXkeyFile string
	ClaimsIssuerFile  string
	Env               map[string]string
	Essential         bool
	DevMode           bool
	TriggerSubjects   []string
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
	BaseImage       string
	BuildScriptPath string
	AgentBinaryPath string
}

// Node configuration is used to configure the node process as well
// as the virtual machines it produces
type NodeOptions struct {
	ConfigFilepath  string `json:"-"`
	ForceDepInstall bool   `json:"-"`

	OtelMetrics         bool   `json:"-"`
	OtelMetricsPort     int    `json:"-"`
	OtelMetricsExporter string `json:"-"`

	Errors []error `json:"errors,omitempty"`
}

func (c *NodeOptions) Validate() bool {
	c.Errors = make([]error, 0)

	// TODO-- add validation

	return len(c.Errors) == 0
}

func GenerateConnectionFromOpts(opts *Options) (*nats.Conn, error) {
	ctxOpts := []natscontext.Option{
		natscontext.WithServerURL(opts.Servers),
		natscontext.WithCreds(opts.Creds),
		natscontext.WithNKey(opts.Nkey),
		natscontext.WithCertificate(opts.TlsCert),
		natscontext.WithKey(opts.TlsKey),
		natscontext.WithCA(opts.TlsCA),
	}

	if opts.TlsFirst {
		ctxOpts = append(ctxOpts, natscontext.WithTLSHandshakeFirst())
	}

	if opts.Username != "" && opts.Password == "" {
		ctxOpts = append(ctxOpts, natscontext.WithToken(opts.Username))
	} else {
		ctxOpts = append(ctxOpts, natscontext.WithUser(opts.Username), natscontext.WithPassword(opts.Password))
	}

	var err error

	exist, _ := fileAccessible(opts.ConfigurationContext)

	if exist && strings.HasSuffix(opts.ConfigurationContext, ".json") {
		opts.Configuration, err = natscontext.NewFromFile(opts.ConfigurationContext, ctxOpts...)
	} else {
		opts.Configuration, err = natscontext.New(opts.ConfigurationContext, !opts.SkipContexts, ctxOpts...)
	}

	if err != nil {
		return nil, err
	}

	conn, err := opts.Configuration.Connect(nats.Name(
		func() string {
			if opts.ConnectionName == "" {
				return "nex"
			}
			return opts.ConnectionName
		}(),
	))
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func fileAccessible(f string) (bool, error) {
	stat, err := os.Stat(f)
	if err != nil {
		return false, err
	}

	if stat.IsDir() {
		return false, fmt.Errorf("is a directory")
	}

	file, err := os.Open(f)
	if err != nil {
		return false, err
	}
	file.Close()

	return true, nil
}
