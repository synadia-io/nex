package main

import (
	"fmt"
	"net/url"
	"reflect"
	"time"

	"github.com/alecthomas/kong"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

var nCLI nexCLI

type nexCLI struct {
	Global  globals         `embed:""`
	Node    nodeOptions     `cmd:"" help:"Interact with execution engine nodes" aliases:"nodes"`
	Run     runOptions      `cmd:"" help:"Run a workload on a target node"`
	Devrun  devRunOptions   `cmd:"" help:"Run a workload locating reasonable defaults (developer mode)" aliases:"yeet"`
	Stop    stopOptions     `cmd:"" help:"Stop a running workload"`
	Monitor monitorOptions  `cmd:"" help:"Monitor the status of events and logs" aliases:"watch"`
	Rootfs  rootfsOptions   `cmd:"" help:"Build custom rootfs" aliases:"fs"`
	Lame    lameDuckOptions `cmd:"" help:"Command a node to enter lame duck mode" aliases:"lameduck"`
	Upgrade upgradeOptions  `cmd:"" help:"Upgrade the NEX CLI to the latest version"`
	Config  configCmd       `cmd:"" help:"View file node configuration"`
}

type globals struct {
	Config kong.ConfigFlag `short:"c" env:"NEX_CONFIG" type:"existingfile" placeholder:"./config.json" group:"Nex Configuration" json:"globals_config"`
	// Namespace for scoping workload requests
	Namespace string `env:"NEX_NAMESPACE" placeholder:"default" group:"Nex Configuration" json:"globals_namespace"`
	// CLI version
	Version kong.VersionFlag `help:"Show version" json:"globals_version"`

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

func (g globals) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("Global Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRow(table.Row{"Config File", g.Config, reflect.TypeOf(g.Config).String()})
	tw.AppendRow(table.Row{"Nex Namespace", g.Namespace, reflect.TypeOf(g.Namespace).String()})
	tw.AppendRow(table.Row{"NATS Server", g.Server, reflect.TypeOf(g.Server).String()})
	tw.AppendRow(table.Row{"NATS Context", g.NatsContext, reflect.TypeOf(g.NatsContext).String()})
	tw.AppendRow(table.Row{"NATS Creds", g.Creds, reflect.TypeOf(g.Creds).String()})
	tw.AppendRow(table.Row{"NATS TLS Cert", g.TlsCert, reflect.TypeOf(g.TlsCert).String()})
	tw.AppendRow(table.Row{"NATS TLS Key", g.TlsKey, reflect.TypeOf(g.TlsKey).String()})
	tw.AppendRow(table.Row{"NATS TLS CA", g.TlsCA, reflect.TypeOf(g.TlsCA).String()})
	tw.AppendRow(table.Row{"NATS TLS First", g.TlsFirst, reflect.TypeOf(g.TlsFirst).String()})
	tw.AppendRow(table.Row{"NATS Timeout", g.Timeout, reflect.TypeOf(g.Timeout).String()})
	tw.AppendRow(table.Row{"NATS Connection Name", g.ConnectionName, reflect.TypeOf(g.ConnectionName).String()})
	tw.AppendRow(table.Row{"NATS Username", g.Username, reflect.TypeOf(g.Username).String()})
	tw.AppendRow(table.Row{"NATS Password", g.Password, reflect.TypeOf(g.Password).String()})
	tw.AppendRow(table.Row{"NATS Nkey", g.Nkey, reflect.TypeOf(g.Nkey).String()})
	tw.AppendRow(table.Row{"NATS Socks Proxy", g.SocksProxy, reflect.TypeOf(g.SocksProxy).String()})
	tw.AppendRow(table.Row{"Logger Writers", g.Logger, reflect.TypeOf(g.Logger).String()})
	tw.AppendRow(table.Row{"Logger Level", g.LogLevel, reflect.TypeOf(g.LogLevel).String()})
	tw.AppendRow(table.Row{"Logger Time Format", g.LogTimeFormat, reflect.TypeOf(g.LogTimeFormat).String()})
	tw.AppendRow(table.Row{"Logger Colorized", g.LogsColorized, reflect.TypeOf(g.LogsColorized).String()})
	tw.AppendRow(table.Row{"Logger JSON", g.LogJSON, reflect.TypeOf(g.LogJSON).String()})

	fmt.Println(tw.Render())
	return nil
}
