package nexcli

import (
	"fmt"

	controlapi "github.com/ConnectEverything/nex/control-api"
	"github.com/choria-io/fisk"
)

func ListNodes(opts *Options) func(*fisk.ParseContext) error {
	nc, err := generateConnectionFromOpts(opts)
	if err != nil {
		return errorClosure(err)
	}
	nodeClient := controlapi.NewApiClient(nc, opts.Timeout)

	return func(ctx *fisk.ParseContext) error {
		nodes, err := nodeClient.ListNodes()
		if err != nil {
			return err
		}
		// TODO
		renderNodeList(nodes)
		return nil
	}
}

func NodeInfo(opts *Options) func(*fisk.ParseContext) error {
	nc, err := generateConnectionFromOpts(opts)
	if err != nil {
		return errorClosure(err)
	}
	nodeClient := controlapi.NewApiClient(nc, opts.Timeout)

	return func(ctx *fisk.ParseContext) error {
		id := ctx.SelectedCommand.Model().Args[0].Value.String()
		nodeInfo, err := nodeClient.NodeInfo(id)
		if err != nil {
			return err
		}
		fmt.Printf("%v", nodeInfo)

		return nil
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
