package main

import (
	"fmt"
	"time"

	"github.com/alecthomas/kong"
)

type Globals struct {
	GlobalLogger `group:"Logger Configuration"`
	GlobalNats   `group:"NATS Configuration"`

	Config    kong.ConfigFlag  `help:"Configuration file to load" placeholder:"./nex.config.json"`
	Version   kong.VersionFlag `help:"Print version information"`
	Namespace string           `env:"NEX_NAMESPACE" placeholder:"default" help:"Specifies namespace when running nex commands" json:"globals_namespace"`
	Check     bool             `optional:"" help:"Print the current configuration"`
}

type GlobalLogger struct {
	Logger         []string `optional:"" default:"std" help:"Logger output targets" enum:"std,file,nats"`
	LogLevel       string   `optional:"" default:"error" short:"l" help:"Set log level" enum:"fatal,error,warn,info,debug,trace"`
	LogJSON        bool     `optional:"" default:"false" help:"Enable JSON formatted logs"`
	LogColor       bool     `optional:"" default:"true" help:"Enable colorized logs"`
	LogShortLevels bool     `optional:"" default:"false" help:"Use abbreviated log levels; DEBUG -> DBG"`
	LogTimeFormat  string   `optional:"" default:"DateTime" help:"Time format for log messages" enum:"DateTime"`
}

type GlobalNats struct {
	NatsContext         string        `optional:"" name:"context" placeholder:"default" help:"NATS context to use for connection; takes priority"`
	NatsServers         []string      `optional:"" name:"servers" short:"s" help:"NATS servers to connect to" default:"nats://localhost:4222"`
	NatsUserNkey        string        `optional:"" name:"nkey" help:"User NKEY file for single-key auth"`
	NatsUserSeed        string        `optional:"" name:"seed" help:"Seed for user credentials" placeholder:"SUNEXSEED..."`
	NatsUserJWT         string        `optional:"" name:"jwt" help:"JWT for user credentials" placeholder:"enexjwtabc123..."`
	NatsUser            string        `optional:"" name:"user" help:"User for credentials" placeholder:"user"`
	NatsUserPassword    string        `optional:"" name:"password" help:"Password for user credentials" placeholder:"password"`
	NatsJSDomain        string        `optional:"" name:"jsdomain" help:"JetStream domain to use" placeholder:"nex"`
	NatsConnectionName  string        `optional:"" name:"conn-name" help:"Connection name to use" default:"nex-${versionOnly}"`
	NatsCredentialsFile string        `optional:"" name:"creds-file" help:"Path to the NATS credentials file" type:"existingfile" placeholder:"/etc/nex/ngs.creds"`
	NatsTimeout         time.Duration `optional:"" name:"timeout" help:"Timeout for NATS operations" placeholder:"5s"`
	NatsTLSCert         string        `optional:"" name:"tlscert" help:"Path to the NATS TLS certificate" type:"existingfile" placeholder:"/etc/nex/tls.crt"`
	NatsTLSKey          string        `optional:"" name:"tlskey" help:"Path to the NATS TLS key" type:"existingfile" placeholder:"/etc/nex/tls.key"`
	NatsTLSCA           string        `optional:"" name:"tlsca" help:"Path to the NATS TLS root CA" type:"existingfile" placeholder:"/etc/nex/ca.crt"`
	NatsTLSFirst        bool          `optional:"" name:"tlsfirst" help:"Enable TLS first" default:"false"`
}

type NexCLI struct {
	Globals Globals `embed:""`

	Node     Node     `cmd:"" help:"Interact with execution engine nodes" alias:"nodes"`
	Workload Workload `cmd:"" help:"Interact with workloads"`
	Monitor  Monitor  `cmd:"" help:"Live monitor workload log emissions"`
	RootFS   RootFS   `cmd:"" name:"rootfs" help:"Build custom rootfs" alias:"fs"`
	Upgrade  Upgrade  `cmd:"" help:"Upgrade the NEX CLI to the latest version"`
}

func (g Globals) Run(ctx *kong.Context) error {
	if g.Check {
		fmt.Println("Check passed")
	} else {
		ctx.Exit(1)
	}
	return nil
}
