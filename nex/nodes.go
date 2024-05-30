package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/nats-io/natscli/columns"
	controlapi "github.com/synadia-io/nex/control-api"
	"github.com/synadia-io/nex/internal/models"
)

// Uses a control API client to ping all nodes in NATS environment
func PingNodes(ctx context.Context) error {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	nc, err := models.GenerateConnectionFromOpts(Opts, log)
	if err != nil {
		return err
	}

	nodeClient := controlapi.NewApiClient(nc, Opts.Timeout, log)
	nodes, err := nodeClient.PingNodes()
	if err != nil {
		return err
	}
	renderNodeList(nodes, NodeOpts.ListFull)

	return nil

}

func ListWorkloads(ctx context.Context) error {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	nc, err := models.GenerateConnectionFromOpts(Opts, log)
	if err != nil {
		return err
	}
	nodeClient := controlapi.NewApiClientWithNamespace(nc, Opts.Timeout, Opts.Namespace, log)
	nodes, err := nodeClient.PingWorkloads(strings.TrimSpace(RunOpts.Name))
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

func renderNodeList(nodes []controlapi.PingResponse, listFull bool) {
	if len(nodes) == 0 {
		fmt.Println("No nodes discovered")
		return
	}

	tbl := newTableWriter("NATS Execution Nodes")
	if !listFull {
		tbl.AddHeaders("ID (* = Lameduck Mode)", "Name", "Version", "Workloads")
	} else {
		tbl.AddHeaders("Nexus", "ID (* = Lameduck Mode)", "Name", "Version", "Workloads", "Uptime", "Sandboxed", "OS", "Arch")
	}

	for _, node := range nodes {
		nodeName, ok := node.Tags["node_name"]
		if !ok {
			nodeName = "no-name"
		}

		ld, ok := node.Tags["nex.lameduck"]
		if !ok {
			ld = "false"
		}
		lameduck, err := strconv.ParseBool(ld)
		if err != nil {
			lameduck = false
		}

		nodeId := func() string {
			if lameduck {
				return node.NodeId + "*"
			}
			return node.NodeId
		}()

		row := []any{nodeId, nodeName, node.Version, node.RunningMachines}

		if listFull {
			nodeUnsafe, ok := node.Tags["nex.unsafe"]
			if !ok {
				nodeUnsafe = "false"
			}
			nUnsafe, _ := strconv.ParseBool(nodeUnsafe)
			nodeOS, ok := node.Tags["nex.os"]
			if !ok {
				nodeOS = "unknown"
			}

			nodeArch, ok := node.Tags["nex.arch"]
			if !ok {
				nodeArch = "unknown"
			}

			row = append(row, node.Uptime, !nUnsafe, nodeOS, nodeArch)
			row = append([]any{node.Nexus}, row...)
		}

		tbl.AddRow(row...)
	}

	tbl.writer.SortBy([]table.SortBy{
		{Name: "Nexus", Mode: table.Asc},
		{Name: "Name", Mode: table.Asc},
	})
	fmt.Println(tbl.Render())
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
