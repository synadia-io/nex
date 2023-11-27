package main

import (
	"context"
	"fmt"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	nexagent "github.com/ConnectEverything/nex/nex-agent"
	"github.com/nats-io/nats.go"
)

func main() {
	ctx := context.Background()
	time.Sleep(1 * time.Second) // give host time to post metadata
	metadata, err := nexagent.GetMachineMetadata()

	if err != nil {
		// If we're using the defaults (e.g. in dev loop) this will at least be able to report
		// the metadata failure
		metadata = &agentapi.MachineMetadata{
			VmId:            "42",
			NodeNatsAddress: "192.168.172.1",
			NodePort:        9222,
			Message:         fmt.Sprintf("%s", err),
		}
	}

	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", metadata.NodeNatsAddress, metadata.NodePort))
	if err != nil {
		panic(err)
	}
	nodeClient, err := nexagent.NewNodeClient(nc, metadata)
	if err != nil {
		panic(err)
	}
	nodeClient.Start()

	<-ctx.Done()
}
