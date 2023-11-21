package nexcli

import (
	"net/url"
	"time"
)

var (
	Opts      = &Options{}
	RunOpts   = &RunOptions{Env: make(map[string]string)}
	StopOpts  = &StopOptions{}
	WatchOpts = &WatchOptions{}
)

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
}

type RunOptions struct {
	TargetNode        string
	WorkloadUrl       *url.URL
	Name              string
	Description       string
	PublisherXkeyFile string
	ClaimsIssuerFile  string
	Env               map[string]string
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
