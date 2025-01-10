//go:build !custom

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/natscli/columns"
	"github.com/nats-io/nkeys"
	"github.com/synadia-labs/nex"
	"github.com/synadia-labs/nex/agents/native"
	"github.com/synadia-labs/nex/client"
	"github.com/synadia-labs/nex/models"
)

type Node struct {
	Up       Up       `cmd:"up" help:"Bring a node up"`
	LameDuck LameDuck `cmd:"lameduck" name:"lameduck" help:"Command a node to enter lame duck mode" aliases:"down"`
	List     List     `cmd:"list" aliases:"ls" help:"List running nodes"`
	Info     Info     `cmd:"info" help:"Provide information about a running node"`
}

type (
	Up struct {
		Agents                 AgentConfigs      `name:"agents" help:"Workload types configurations for nex node to initialize"`
		DisableNativeStart     bool              `name:"disable-native-start" help:"Disable native start agent" default:"false"`
		ShowWorkloadLogs       bool              `name:"show-workload-logs" help:"Hide logs from workloads" default:"false"`
		NexusName              string            `name:"nexus" default:"nexus" help:"Nexus name"`
		NodeName               string            `name:"node-name" placeholder:"nex-node" help:"Name of the node; random if not provided"`
		NodeSeed               string            `name:"node-seed" help:"Node Seed used for identifier.  Default is generated" placeholder:"NBTAFHAKW..."`
		NodeXKeySeed           string            `name:"node-xkey-seed" help:"Node XKey Seed used for encryption.  Default is generated" placeholder:"XAIHERHS..."`
		ResourceDir            string            `name:"resource-directory" default:"${defaultResourcePath}"`
		Tags                   map[string]string `name:"tags" placeholder:"nex:iscool;..." help:"Tags to be used for nex node"`
		NoState                bool              `name:"no-state" help:"Disable state persistence" default:"false"`
		InternalNatsServerConf string            `name:"inats-config" help:"Path to the NATS configuration file" type:"existingfile" placeholder:"/etc/nex/nats.conf"`
		SigningKey             string            `name:"signing-key" help:"Seed key for signing" placeholder:"SASIGNINGKEY..."`
		RootAccountKey         string            `name:"root-account-key" help:"Public key for root account" placeholder:"AAMYACCOUNT..."`
	}
	Info struct {
		NodeID string `arg:"node-id" required:"" help:"Node ID to query" placeholder:"NBTAFHAKW..."`
	}
	LameDuck struct {
		Delay  time.Duration     `name:"delay" help:"Delay before stopping workloads.  Allows for user to migrate workloads" default:"1m"`
		Label  map[string]string `name:"label" help:"Put all nodes with label in lameduck.  Only 1 label allowed" placeholder:"nex.nexus=mynexus"`
		NodeID string            `name:"node-id" arg:"" help:"Node ID to command into lame duck mode" placeholder:"NBTAFHAKW..."`
	}
	List struct {
		Filter map[string]string `name:"filter" help:"Filter the list of nodes on tags. Node must match all provided tags to be returned" placeholder:"nex.nexus=mynexus"`
	}
)

func (u Up) Validate() error {
	var errs error

	if u.NodeSeed != "" {
		prefix, _, err := nkeys.DecodeSeed([]byte(u.NodeSeed))
		if err != nil {
			errs = errors.Join(errs, err)
		} else {
			if prefix != nkeys.PrefixByteServer {
				errs = errors.Join(errs, errors.New("node seed must be a server seed"))
			}
		}
	}

	if u.SigningKey != "" && u.RootAccountKey == "" {
		errs = errors.Join(errs, errors.New("root-account-key must be provided if signing-key is provided"))
	}

	if u.RootAccountKey != "" && u.SigningKey == "" {
		errs = errors.Join(errs, errors.New("signing-key must be provided if root-account-key is provided"))
	}

	if u.NodeXKeySeed != "" {
		_, err := nkeys.Decode(nkeys.PrefixByteCluster, []byte(u.NodeXKeySeed))
		errs = errors.Join(errs, err)
	}

	if u.InternalNatsServerConf != "" {
		_, err := server.ProcessConfigFile(u.InternalNatsServerConf)
		errs = errors.Join(errs, err)
	}

	return errs
}

func (u Up) Run(ctx context.Context, globals *Globals) error {
	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	if nc == nil && u.InternalNatsServerConf == "" {
		return errors.New("no NATS connection available, and no internal NATS server configuration provided")
	}

	var nodeKeyPair nkeys.KeyPair
	if u.NodeSeed != "" {
		nodeKeyPair, err = nkeys.FromSeed([]byte(u.NodeSeed))
		if err != nil {
			return err
		}
	} else {
		nodeKeyPair, err = nkeys.CreateServer()
		if err != nil {
			return err
		}
	}

	var nodeXkeyPair nkeys.KeyPair
	if u.NodeXKeySeed != "" {
		nodeXkeyPair, err = nkeys.FromSeed([]byte(u.NodeXKeySeed))
		if err != nil {
			return err
		}
	} else {
		nodeXkeyPair, err = nkeys.CreateCurveKeys()
		if err != nil {
			return err
		}
	}

	nodePub, err := nodeKeyPair.PublicKey()
	if err != nil {
		return err
	}

	logger := configureLogger(globals, nc, nodePub, u.ShowWorkloadLogs)
	if nc == nil && slices.Contains(globals.Target, "nats") {
		logger.Warn("No NATS connection available, logs will not be sent over NATS")
	}

	var natsUrl string
	if nc != nil {
		natsUrl = nc.ConnectedUrl()
	} else {
		natsOpts, err := server.ProcessConfigFile(u.InternalNatsServerConf)
		if err != nil {
			return err
		}
		natsUrl = fmt.Sprintf("nats://%s:%d", natsOpts.Host, natsOpts.Port)
	}

	opts := []nex.NexNodeOption{
		nex.WithNodeName(u.NodeName),
		nex.WithNexus(u.NexusName),
		nex.WithNatsConn(nc),
		nex.WithLogger(logger),
		nex.WithNodeKeyPair(nodeKeyPair),
		nex.WithNodeXKeyPair(nodeXkeyPair),
	}

	if u.NoState {
		opts = append(opts, nex.WithNoState())
	}

	if !u.DisableNativeStart {
		nativeAgent, err := native.NewNativeWorkloadAgent(natsUrl, nodePub, logger.WithGroup("native-agent"))
		if err != nil {
			return err
		}
		opts = append(opts, nex.WithAgentRunner(nativeAgent))
	}

	if u.InternalNatsServerConf != "" {
		natsOpts, err := server.ProcessConfigFile(u.InternalNatsServerConf)
		if err != nil {
			return err
		}
		opts = append(opts, nex.WithInternalNatsServer(natsOpts))
	}

	for _, agent := range u.Agents {
		opts = append(opts, nex.WithLocalAgent(models.Agent{
			Uri:  agent.Uri,
			Argv: agent.Argv,
			Env:  agent.Env,
		}))
	}

	for k, v := range u.Tags {
		opts = append(opts, nex.WithTag(k, v))
	}

	if u.SigningKey != "" && u.RootAccountKey != "" {
		opts = append(opts, nex.WithSigningKey(u.SigningKey, u.RootAccountKey))
	}

	nex, err := nex.NewNexNode(opts...)
	if err != nil {
		return err
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	go func() {
		select {
		case <-ctx.Done():
			logger.Warn("shutting down due to cancelled context")
			err := nex.Shutdown()
			if err != nil {
				logger.Error("Error shutting down nex", "error", err)
			}
		case <-quit:
			err := nex.Shutdown()
			if err != nil {
				logger.Error("Error shutting down nex", "error", err)
			}
		}
	}()

	if err := nex.Start(); err != nil {
		return err
	}

	return nex.WaitForShutdown()
}

func (i Info) Run(ctx context.Context, globals *Globals) error {
	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	if nc == nil {
		return errors.New("no NATS connection available")
	}

	infoResponse, err := client.NewClient(nc, globals.Namespace).GetNodeInfo(i.NodeID)
	if err != nil {
		return err
	}

	if globals.JSON {
		infoB, err := json.Marshal(infoResponse)
		if err != nil {
			return err
		}
		fmt.Println(string(infoB))
		return nil
	}

	tags := make([]string, 0)
	for k, v := range infoResponse.Tags {
		tags = append(tags, fmt.Sprintf("%s=%s", k, v))
	}

	w := columns.New("Information about Node %s", infoResponse.NodeId)
	w.AddRow("Nexus", infoResponse.Tags[models.TagNexus])
	w.AddRow("Node Name", infoResponse.Tags[models.TagNodeName])
	w.AddRow("Tags", tags)
	w.AddRow("XKey", infoResponse.Xkey)
	w.AddRow("Uptime", infoResponse.Uptime)
	w.AddRow("Version", infoResponse.Version)
	details, err := w.Render()
	if err != nil {
		return err
	}

	if len(infoResponse.AgentSummaries) > 0 {
		tW := table.NewWriter()
		tW.SetStyle(table.StyleRounded)
		tW.Style().Title.Align = text.AlignCenter
		tW.Style().Format.Header = text.FormatDefault
		tW.SetTitle("Running Agents")

		tW.AppendHeader(table.Row{"Id", "Name", "Start Time", "State", "Supported Lifecycles", "Running Workloads"})
		for aId, aInfo := range infoResponse.AgentSummaries {
			tW.AppendRow(table.Row{aId, aInfo.Name, aInfo.StartTime, aInfo.State, aInfo.SupportedLifecycles, aInfo.WorkloadCount})
		}

		tW.SortBy([]table.SortBy{
			{Name: "Name", Mode: table.Asc},
		})

		fmt.Println(details)
		fmt.Println(tW.Render())
	} else {
		fmt.Println(details)
		fmt.Println("No agents running")
	}
	return nil
}

func (l LameDuck) Run(ctx context.Context, globals *Globals) error {
	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	if nc == nil {
		return errors.New("no NATS connection available")
	}

	ldr, err := client.NewClient(nc, globals.Namespace).SetLameduck(l.NodeID, l.Delay)
	if err != nil {
		return err
	}

	if globals.JSON {
		respB, err := json.Marshal(ldr)
		if err != nil {
			return err
		}
		fmt.Println(string(respB))
		return nil
	}

	if ldr.Success {
		fmt.Println("Node successfully commanded into lame duck mode. Workloads will being shutting down at", time.Now().Add(l.Delay).Format(time.RFC3339))
		return nil
	}

	fmt.Println("Node failed to enter lame duck mode")
	return nil
}

func (l List) Run(ctx context.Context, globals *Globals) error {
	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	if nc == nil {
		return errors.New("no NATS connection available")
	}

	resp, err := client.NewClient(nc, globals.Namespace).ListNodes(l.Filter)
	if err != nil {
		return err
	}

	if globals.JSON {
		respB, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		fmt.Println(string(respB))
		return nil
	}

	if len(resp) > 0 {
		tW := table.NewWriter()

		tW.SetTitle("Nex Nodes")
		tW.SetStyle(table.StyleRounded)
		tW.Style().Title.Align = text.AlignCenter
		tW.Style().Format.Header = text.FormatDefault

		tW.AppendHeader(table.Row{"Nexus", "ID (* = Lameduck Mode)", "Name", "Uptime", "State", "Running Agents"})
		for _, nInfo := range resp {
			nexus, ok := nInfo.Tags[models.TagNexus]
			if !ok {
				nexus = "[unknown]"
			}
			name, ok := nInfo.Tags[models.TagNodeName]
			if !ok {
				name = "[unknown]"
			}

			id := nInfo.NodeId
			ld, ok := nInfo.Tags[models.TagLameDuck]
			if ok && ld == "true" {
				id = id + "*"
			}

			tW.AppendRow(table.Row{nexus, id, name, nInfo.Uptime, nInfo.State, nInfo.AgentCount})
		}

		tW.SortBy([]table.SortBy{
			{Name: "Nexus", Mode: table.Asc},
			{Name: "Name", Mode: table.Asc},
		})

		fmt.Println(tW.Render())
		return nil
	}
	fmt.Println("No nodes found")
	return nil
}
