package main

import (
	"context"
	"fmt"
	"time"

	nexagent "github.com/ConnectEverything/nex/nex-agent"
	"github.com/nats-io/nats.go"
)

func main() {
	ctx := context.Background()
	time.Sleep(1 * time.Second) // give host time to post metadata
	metadata, err := nexagent.GetMachineMetadata()

	if err != nil {
		nexagent.HaltVM(err)
	}

	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", metadata.NodeNatsAddress, metadata.NodePort))
	if err != nil {
		nexagent.HaltVM(err)
	}
	nodeClient, err := nexagent.NewNodeClient(nc, metadata)
	if err != nil {
		nexagent.HaltVM(err)
	}
	nodeClient.Start()

	<-ctx.Done()
}
