package main

import (
	"net/url"
	"time"

	"github.com/alecthomas/kong"
)

type globals struct {
	Config kong.ConfigFlag `short:"c" env:"NEX_CONFIG" type:"existingfile" placeholder:"./config.json" group:"Nex Configuration" json:"globals_config"`
	// Namespace for scoping workload requests
	Namespace string `env:"NEX_NAMESPACE" default:"default" group:"Nex Configuration" json:"globals_namespace"`
	// CLI version
	Version kong.VersionFlag `help:"Show version" group:"Nex Configuration" json:"globals_version"`
	// Prints final coniguration of the CLI
	Check bool `default:"false" help:"Prints final configuration of command without running" group:"Nex Configuration" json:"globals_check"`

	// Address of the NATS server
	Server string `env:"NEX_NATS_SERVER" default:"${default_nats_server}" group:"NATS Configuration" json:"globals_nats_server"`
	// Name or path to a configuration context
	NatsContext string `env:"NEX_NATS_CONTEXT" placeholder:"local" group:"NATS Configuration" json:"globals_nats_context"`
	// Creds is nats credentials to authenticate with
	Creds string `env:"NEX_NATS_CREDS" type:"existingfile" placeholder:"./path/to/creds.file" group:"NATS Configuration" json:"globals_nats_creds"`
	// TlsCert is the TLS Public Certificate
	TlsCert string `env:"NEX_NATS_TLSCERT" type:"existingfile" placeholder:"./path/to/cert" group:"NATS Configuration" json:"globals_nats_tlscert"`
	// TlsKey is the TLS Private Key
	TlsKey string `env:"NEX_NATS_TLSKEY" type:"existingfile" placeholder:"./path/to/cert.key" group:"NATS Configuration" json:"globals_nats_tlskey"`
	// TlsCA is the certificate authority to verify the connection with
	TlsCA string `env:"NEX_NATS_TLSCA" type:"existingfile" placeholder:"./path/to/ca" group:"NATS Configuration" json:"globals_nats_tlsca"`
	// TlsFirst configures theTLSHandshakeFirst behavior in nats.go
	TlsFirst bool `env:"NEX_NATS_TLSFIRST" group:"NATS Configuration" json:"globals_nats_tlsfirst"`
	// Timeout is how long to wait for operations
	Timeout time.Duration `env:"NEX_NEX_TIMEOUT" default:"3s" group:"NATS Configuration" json:"globals_nats_timeout"`
	// ConnectionName is the name to use for the underlying NATS connection
	ConnectionName string `env:"NEX_NATS_CONN_NAME" default:"${default_nats_conn_name}" group:"NATS Configuration" json:"globals_nats_conn_name"`
	// Username is the username or token to connect with
	Username string `env:"NEX_NATS_USER" placeholder:"bob" group:"NATS Configuration" json:"globals_nats_username"`
	// Password is the password to connect with
	Password string `env:"NEX_NATS_PASSWORD" placeholder:"secretz" group:"NATS Configuration" json:"globals_nats_password"`
	// Nkey is the file holding a nkey to connect with
	Nkey string `env:"NEX_NATS_NKEY" placeholder:"UNEXROX..." group:"NATS Configuration" json:"globals_nats_nkey"`
	// SocksProxy is a SOCKS5 proxy to use for NATS connections
	SocksProxy *url.URL `env:"NEX_NATS_SOCKS_PROXY" placeholder:"127.0.0.1" group:"NATS Configuration" json:"globals_nats_socks_proxy"`
	// Type of logger
	Logger []string `env:"NEX_LOGGER" default:"std" enum:"std,file,nats" group:"Logger Configuration" help:"Options: std, file, nats" json:"globals_logger"`
	// LogLevel is the log level to use
	LogLevel string `env:"NEX_LOG_LEVEL" default:"info" enum:"debug,warn,info,error" group:"Logger Configuration" help:"Options: debug, warn, info, error" json:"globals_logger_level"`
	// LogJSON enables JSON logging
	LogTimeFormat string `env:"NEX_LOG_TIME_FORMAT" default:"DateTime" enum:"DateOnly,DateTime,Stamp,RFC822,RFC3339" group:"Logger Configuration" help:"Options: DateOnly, DateTime, Stamp, RFC822, RFC3339" json:"globals_logger_time_format"`
	// Timeformat for logs.  Must satisfy time.Time format criteria
	LogsColorized bool `env:"NEX_LOG_COLORIZED" default:"false" group:"Logger Configuration" json:"globals_logger_color"`
	// Outputs logs with color
	LogJSON bool `env:"NEX_LOG_JSON" default:"false" group:"Logger Configuration" json:"globals_logger_json"`
}
