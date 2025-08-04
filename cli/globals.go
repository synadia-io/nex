package main

import (
	"time"

	"github.com/alecthomas/kong"
)

type Globals struct {
	GlobalLogger `prefix:"logger." group:"Logger Configuration"`
	GlobalNats   `prefix:"nats." group:"NATS Configuration"`

	Config              kong.ConfigFlag  `help:"Configuration file to load" placeholder:"./nex.json"`
	Version             kong.VersionFlag `help:"Print version information"`
	Namespace           string           `name:"namespace" env:"NEX_NAMESPACE" default:"${adminNamespace}" help:"Specifies namespace when running nex commands"`
	Check               bool             `name:"check" help:"Print the current values of all options without running a command"`
	DisableUpgradeCheck bool             `name:"disable-upgrade-check" env:"NEX_DISABLE_UPGRADE_CHECK" help:"Disable the upgrade check"`
	AutoUpgrade         bool             `name:"auto-upgrade" env:"NEX_AUTO_UPGRADE" help:"Automatically upgrade the nex CLI when a new version is available"`
	DevMode             bool             `name:"dev-mode" default:"false" help:"Enable development mode"`
	JSON                bool             `name:"json" help:"Displays any output in JSON format"`
}

type GlobalLogger struct {
	Target          []string `name:"target" default:"std" help:"Logger output targets" enum:"std,file,nats"`
	LogLevel        string   `name:"level" default:"info" short:"l" help:"Set log level" enum:"fatal,error,warn,info,debug,trace"`
	LogColor        bool     `name:"color" default:"false" help:"Enable colorized logs"`
	LogShortLevels  bool     `name:"short" default:"false" help:"Use abbreviated log levels; DEBUG -> DBG"`
	LogTimeFormat   string   `name:"timefmt" default:"DateTime" help:"Time format for log messages" enum:"DateTime,TimeOnly,DateOnly,Stamp,RFC822,RFC3339"`
	LogWithPid      bool     `name:"with-pid" default:"false" help:"Include process ID in log messages"`
	LogGroupOnRight bool     `name:"group-on-right" default:"false" help:"Place log group on the right"`
	LogLineInfo     bool     `name:"line-info" default:"false" help:"Include file and line number in log messages"`
}

type GlobalNats struct {
	NatsContext         string        `name:"context" placeholder:"<context>" help:"NATS context to use for connection; takes priority"`
	NatsServers         []string      `name:"servers" short:"s" help:"NATS servers to connect to" placeholder:"nats://127.0.0.1:4222"`
	NatsUserNkey        string        `name:"nkey" help:"User NKEY file for single-key auth"`
	NatsUserSeed        string        `name:"seed" help:"Seed for user credentials" placeholder:"SUNEXSEED..."`
	NatsUserJWT         string        `name:"jwt" help:"JWT for user credentials" placeholder:"enexjwtabc123..."`
	NatsUser            string        `name:"user" help:"User for credentials" placeholder:"user"`
	NatsUserPassword    string        `name:"password" help:"Password for user credentials" placeholder:"password"`
	NatsJSDomain        string        `name:"jsdomain" help:"JetStream domain to use" placeholder:"nex"`
	NatsConnectionName  string        `name:"conn-name" help:"Connection name to use" default:"nex-${versionOnly}"`
	NatsCredentialsFile string        `name:"creds-file" help:"Path to the NATS credentials file" type:"existingfile" placeholder:"/etc/nex/ngs.creds"`
	NatsTimeout         time.Duration `name:"timeout" help:"Timeout for NATS operations" placeholder:"5s"`
	NatsTLSCert         string        `name:"tlscert" help:"Path to the NATS TLS certificate" type:"existingfile" placeholder:"/etc/nex/tls.crt"`
	NatsTLSKey          string        `name:"tlskey" help:"Path to the NATS TLS key" type:"existingfile" placeholder:"/etc/nex/tls.key"`
	NatsTLSCA           string        `name:"tlsca" help:"Path to the NATS TLS root CA" type:"existingfile" placeholder:"/etc/nex/ca.crt"`
	NatsTLSFirst        bool          `name:"tlsfirst" help:"Enable TLS first" default:"false"`
}
