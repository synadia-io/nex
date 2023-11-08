package nexcli

import (
	"fmt"
	"os"
	"strings"

	controlapi "github.com/ConnectEverything/nex/control-api"
	"github.com/choria-io/fisk"
)

func ListNodes(ctx *fisk.ParseContext) error {

	nc, err := generateConnectionFromOpts()
	if err != nil {
		return err
	}
	nodeClient := controlapi.NewApiClient(nc, Opts.Timeout)

	nodes, err := nodeClient.ListNodes()
	if err != nil {
		return err
	}
	// TODO
	renderNodeList(nodes)
	return nil

}

func NodeInfo(ctx *fisk.ParseContext) error {

	nc, err := generateConnectionFromOpts()
	if err != nil {
		return err
	}
	nodeClient := controlapi.NewApiClient(nc, Opts.Timeout)
	id := ctx.SelectedCommand.Model().Args[0].Value.String()
	nodeInfo, err := nodeClient.NodeInfo(id)
	if err != nil {
		return err
	}
	renderNodeInfo(nodeInfo, id)

	return nil
}

func renderNodeInfo(info *controlapi.InfoResponse, id string) {
	cols := newColumns("NEX Node Information")
	defer cols.Frender(os.Stdout)
	cols.AddRow("Node", id)
	cols.AddRowf("Xkey", info.PublicXKey)
	cols.AddRow("Version", info.Version)
	cols.AddRow("Uptime", info.Uptime)

	taglist := make([]string, 0)
	for k, v := range info.Tags {
		taglist = append(taglist, fmt.Sprintf("%s=%s", k, v))
	}
	cols.AddRow("Tags", strings.Join(taglist, ", "))

	cols.AddSectionTitle("Workloads:")
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
