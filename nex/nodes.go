package main

import (
	"fmt"
	"os"
	"strings"

	controlapi "github.com/ConnectEverything/nex/internal/control-api"
	"github.com/choria-io/fisk"
	"github.com/nats-io/natscli/columns"
	"github.com/sirupsen/logrus"
)

// Uses a control API client to request a node list from a NATS environment
func ListNodes(ctx *fisk.ParseContext) error {

	nc, err := generateConnectionFromOpts()
	if err != nil {
		return err
	}
	log := logrus.New()
	nodeClient := controlapi.NewApiClient(nc, Opts.Timeout, log)

	nodes, err := nodeClient.ListNodes()
	if err != nil {
		return err
	}

	renderNodeList(nodes)
	return nil

}

// Uses a control API client to retrieve info on a single node
func NodeInfo(ctx *fisk.ParseContext) error {

	nc, err := generateConnectionFromOpts()
	if err != nil {
		return err
	}
	log := logrus.New()
	nodeClient := controlapi.NewApiClientWithNamespace(nc, Opts.Timeout, Opts.Namespace, log)
	id := ctx.SelectedCommand.Model().Args[0].Value.String()
	nodeInfo, err := nodeClient.NodeInfo(id)
	if err != nil {
		return err
	}
	renderNodeInfo(nodeInfo, id)

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
			cols.AddRow("Runtime", m.Uptime)
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
	table.AddHeaders("ID", "Version", "Uptime", "Workloads")

	for _, node := range nodes {
		table.AddRow(node.NodeId, node.Version, node.Uptime, node.RunningMachines)
	}

	fmt.Println(table.Render())
}
