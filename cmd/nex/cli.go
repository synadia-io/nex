package main

import (
	"fmt"
	"time"

	"github.com/alecthomas/kong"
)

type GlobalLogger struct {
	Target         []string `default:"std" help:"Logger output targets" enum:"std,file,nats"`
	LogLevel       string   `name:"level" default:"error" short:"l" help:"Set log level" enum:"fatal,error,warn,info,debug,trace"`
	LogJSON        bool     `name:"json" default:"false" help:"Enable JSON formatted logs"`
	LogColor       bool     `name:"color" default:"true" help:"Enable colorized logs"`
	LogShortLevels bool     `name:"short" default:"false" help:"Use abbreviated log levels; DEBUG -> DBG"`
	LogTimeFormat  string   `name:"timefmt" default:"DateTime" help:"Time format for log messages" enum:"DateTime"`
}

type GlobalNats struct {
	NatsContext         string        `name:"context" placeholder:"default" help:"NATS context to use for connection; takes priority"`
	NatsServers         []string      `name:"servers" short:"s" help:"NATS servers to connect to" default:"nats://localhost:4222"`
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

func (g Globals) Run(ctx *kong.Context) error {
	if g.Check {
		fmt.Println("Check passed")
	} else {
		ctx.Exit(1)
	}
	return nil
}
