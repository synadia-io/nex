package main

import (
	"log"
	"net"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	nexagent "github.com/ConnectEverything/nex/nex-agent"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	s := grpc.NewServer()
	agentapi.RegisterNexAgentServer(s, nexagent.NewApiServer())
	reflection.Register(s)

	lis, err := net.Listen("tcp", ":8081")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	go func() {
		time.Sleep(2 * time.Second)
		nexagent.PublishAgentStarted()
	}()

	s.Serve(lis)

	// Can't publish agent stopped here since `s` is not running
}
