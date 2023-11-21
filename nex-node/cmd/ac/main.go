package main

import (
	"context"
	"fmt"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

func main() {
	ctx := context.Background()
	ac := agentapi.NewAgentClient("0.0.0.0:8081")
	logs, err := ac.SubscribeToLogs()
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	go func() {
		for {
			entry := <-logs
			fmt.Printf("%v\n", entry)
		}
	}()
	evts, err := ac.SubscribeToEvents()
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	go func() {
		for {
			event := <-evts
			fmt.Printf("%v\n", event)
		}
	}()

	fmt.Println("Submitting workload")

	ack, err := ac.PostWorkload("workload", "/home/kevin/lab/firecracker/testworkload/workload", make(map[string]string))
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	if !ack.Error {
		fmt.Println("YAY")
	} else {
		fmt.Printf("BOO: %s\n", ack.Message)
	}

	<-ctx.Done()
}
