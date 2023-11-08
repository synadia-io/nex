package nexagent

import (
	"context"
	"fmt"
	"io"
	"os"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

var (
	agentLogs chan (*agentapi.LogEntry)   = make(chan *agentapi.LogEntry)
	eventLogs chan (*agentapi.AgentEvent) = make(chan *agentapi.AgentEvent)
)

type AgentApiServer struct {
	agentapi.UnimplementedNexAgentServer
}

func (api *AgentApiServer) GetHealth(ctx context.Context, in *agentapi.Void) (*agentapi.HealthReply, error) {
	return &agentapi.HealthReply{
		Healthy:      true,
		AgentVersion: VERSION,
	}, nil
}

func (api *AgentApiServer) SubscribeToLogs(in *agentapi.Void, stream agentapi.NexAgent_SubscribeToLogsServer) error {
	for {
		entry := <-agentLogs
		err := stream.Send(entry)
		if err == io.EOF {
			// client is no longer listening
			break
		}
		if err != nil {
			// TODO: log this
			return err
		}
	}
	return nil
}

func (api *AgentApiServer) SubscribeToEvents(in *agentapi.Void, stream agentapi.NexAgent_SubscribeToEventsServer) error {
	for {
		entry := <-eventLogs
		err := stream.Send(entry)
		if err == io.EOF {
			// client is no longer listening
			break
		}
		if err != nil {
			// TODO: log this
			return err
		}
	}
	return nil
}

func (api *AgentApiServer) SubmitWorkload(stream agentapi.NexAgent_SubmitWorkloadServer) error {
	var hdr *agentapi.WorkloadMetadata
	totalBytesReceived := 0
	tempFile, err := os.CreateTemp("", "sus-")
	if err != nil {
		return stream.SendAndClose(&agentapi.WorkloadAck{Error: true, Message: fmt.Sprintf("Failed to create temp file: %s", err)})
	}
	err = tempFile.Chmod(0777)
	if err != nil {
		return stream.SendAndClose(&agentapi.WorkloadAck{Error: true, Message: fmt.Sprintf("Failed to chmod temp file to executable: %s", err)})
	}

	for {
		workload, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			msg := fmt.Sprintf("Unexpected stream error receiving workload: %s", err)
			LogError(msg)
			return stream.SendAndClose(&agentapi.WorkloadAck{Error: true, Message: msg})
		}

		switch chunk := workload.ChunkType.(type) {
		case *agentapi.Workload_Chunk:
			_, err = tempFile.Write(chunk.Chunk)
			if err != nil {
				return stream.SendAndClose(&agentapi.WorkloadAck{Error: true, Message: fmt.Sprintf("Failed to write file chunk: %s", err)})
			}
			totalBytesReceived += len(chunk.Chunk)
		case *agentapi.Workload_Header:
			agentLogs <- &agentapi.LogEntry{
				Source: "nex-agent",
				Level:  agentapi.LogLevel_LEVEL_DEBUG,
				Text:   fmt.Sprintf("Received workload header %s", chunk.Header.Name),
			}
			hdr = chunk.Header

		default:
			return stream.SendAndClose(&agentapi.WorkloadAck{Error: true, Message: "Protobuf failure - unknown chunk type"})
		}
	}

	if totalBytesReceived < int(hdr.TotalBytes) {
		defer os.Remove(tempFile.Name())
		msg := fmt.Sprintf("Only received %d of expected %d bytes of workload file", totalBytesReceived, hdr.TotalBytes)
		LogError(msg)
		return stream.SendAndClose(&agentapi.WorkloadAck{Error: true, Message: msg})
	}

	err = tempFile.Close()
	if err != nil {
		defer os.Remove(tempFile.Name())
		msg := fmt.Sprintf("Failed to close tempfile: %s", err)
		LogError(msg)
		return stream.SendAndClose(&agentapi.WorkloadAck{Error: true, Message: msg})
	}

	err = RunWorkload(hdr.Name, hdr.TotalBytes, tempFile, hdr.RuntimeEnvironment)
	if err != nil {
		return stream.SendAndClose(&agentapi.WorkloadAck{Error: true, Message: fmt.Sprintf("Failed to await program execution: %s", err)})
	}

	return stream.SendAndClose(&agentapi.WorkloadAck{Error: false, Message: "OK"})
}
