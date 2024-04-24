package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/nats-io/natscli/columns"
	controlapi "github.com/synadia-io/nex/control-api"
	"github.com/synadia-io/nex/internal/models"
)

// Uses a control API client to request a node list from a NATS environment
func ListNodes(ctx context.Context) error {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	nc, err := models.GenerateConnectionFromOpts(Opts, log)
	if err != nil {
		return err
	}

	nodeClient := controlapi.NewApiClient(nc, Opts.Timeout, log)
	nodes, err := nodeClient.ListAllNodes()
	if err != nil {
		return err
	}
	renderNodeList(nodes)

	return nil

}

func ListWorkloads(ctx context.Context) error {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	nc, err := models.GenerateConnectionFromOpts(Opts, log)
	if err != nil {
		return err
	}
	nodeClient := controlapi.NewApiClientWithNamespace(nc, Opts.Timeout, Opts.Namespace, log)
	nodes, err := nodeClient.ListWorkloads(strings.TrimSpace(RunOpts.Name))
	if err != nil {
		return err
	}
	renderWorkloadPingList(nodes)

	return nil
}

func LameDuck(ctx context.Context, logger *slog.Logger) error {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	nodeId := RunOpts.TargetNode
	nc, err := models.GenerateConnectionFromOpts(Opts, log)
	if err != nil {
		return err
	}
	nodeClient := controlapi.NewApiClientWithNamespace(nc, Opts.Timeout, Opts.Namespace, log)
	_, err = nodeClient.EnterLameDuck(nodeId)
	if err != nil {
		fmt.Printf("Failed to issue lame duck command: %s\n", err)
		return nil
	}
	fmt.Printf("Command to enter lame duck mode issued to %s\n", nodeId)

	return nil
}

// Uses a control API client to retrieve info on a single node
func NodeInfo(ctx context.Context, nodeid string) error {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	nc, err := models.GenerateConnectionFromOpts(Opts, log)
	if err != nil {
		return err
	}
	nodeClient := controlapi.NewApiClientWithNamespace(nc, Opts.Timeout, Opts.Namespace, log)
	nodeInfo, err := nodeClient.NodeInfo(nodeid)
	if err != nil {
		return err
	}
	renderNodeInfo(nodeInfo, nodeid)

	return nil
}

func render(cols *columns.Writer) {
	_ = cols.Frender(os.Stdout)
}
func renderNodeInfo(info *controlapi.InfoResponse, id string) {
	cols := newColumns("NEX Node Information")

	defer render(cols)
	cols.AddRow("Node", id)
	cols.AddRowf("Xkey", info.PublicXKey)
	cols.AddRow("Version", info.Version)
	cols.AddRow("Uptime", info.Uptime)

	taglist := make([]string, 0)
	for k, v := range info.Tags {
		taglist = append(taglist, fmt.Sprintf("%s=%s", k, v))
	}
	cols.AddRow("Tags", strings.Join(taglist, ", "))

	if info.Memory != nil {
		cols.AddSectionTitle("Memory in kB")
		cols.Indent(2)

		cols.Println()
		cols.AddRow("Free", info.Memory.MemFree)
		cols.AddRow("Available", info.Memory.MemAvailable)
		cols.AddRow("Total", info.Memory.MemTotal)

		cols.Indent(0)
	}

	if len(info.Machines) > 0 {
		cols.AddSectionTitle("Workloads")
		cols.Indent(2)
		for _, m := range info.Machines {
			cols.Println()
			cols.AddRow("Id", m.Id)
			cols.AddRow("Healthy", m.Healthy)
			cols.AddRow("Runtime", m.Workload.Runtime)
			cols.AddRow("Name", m.Workload.Name)
			cols.AddRow("Description", m.Workload.Description)
		}
		cols.Indent(0)
	}
}

func renderNodeList(nodes []controlapi.PingResponse) {
	if len(nodes) == 0 {
		fmt.Println("No nodes discovered")
		return
	}

	table := newTableWriter("NATS Execution Nodes")
	table.AddHeaders("ID", "Name", "Sandbox Enabled", "Version", "Uptime", "Workloads")

	for _, node := range nodes {
		nodeName, ok := node.Tags["node_name"]
		if !ok {
			nodeName = "no-name"
		}

		nodeUnsafe, ok := node.Tags["nex.unsafe"]
		if !ok {
			nodeUnsafe = "false"
		}
		nUnsafe, _ := strconv.ParseBool(nodeUnsafe)
		table.AddRow(node.NodeId, nodeName, !nUnsafe, node.Version, node.Uptime, node.RunningMachines)
	}

	fmt.Println(table.Render())
}

func renderWorkloadPingList(nodes []controlapi.WorkloadPingResponse) {
	if len(nodes) == 0 {
		fmt.Println("No workloads matched")
		return
	}

	table := newTableWriter("Discovered Workloads")
	table.AddHeaders("ID", "Name", "Type", "Namespace", "Node Name")

	for _, node := range nodes {
		nodeName, ok := node.Tags["node_name"]
		if !ok {
			nodeName = "no-name"
		}
		for _, work := range node.RunningMachines {
			table.AddRow(work.Id, work.Name, work.WorkloadType, work.Namespace, nodeName)
		}
	}

	fmt.Println(table.Render())
}
