package agentapi

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/imroc/req"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// AgentClient is used to communicate with the NEX agent running inside a firecracker VM
type AgentClient struct {
	ip     string
	client NexAgentClient
}

func NewAgentClient(ip string) AgentClient {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.Dial(ip, opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	return AgentClient{
		ip:     ip,
		client: NewNexAgentClient(conn),
	}
}

func (c AgentClient) SubscribeToLogs() (<-chan (*LogEntry), error) {
	ctx := context.Background()
	ch := make(chan *LogEntry)
	stream, err := c.client.SubscribeToLogs(ctx, &Void{})
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			entry, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				// TODO: log this
				return
			}
			ch <- entry
		}
	}()
	return ch, nil
}

func (c AgentClient) SubscribeToEvents() (<-chan (*AgentEvent), error) {
	ctx := context.Background()
	ch := make(chan *AgentEvent)
	stream, err := c.client.SubscribeToEvents(ctx, &Void{})
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			entry, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				// TODO: log this
				return
			}
			ch <- entry
		}
	}()
	return ch, nil
}

func (c AgentClient) Health() (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	health, err := c.client.GetHealth(ctx, &Void{})
	if err != nil {
		return false, "", err
	}
	return health.Healthy, health.AgentVersion, nil
}

func (c AgentClient) PostWorkload(workloadPath string, env map[string]string) (*WorkloadAck, error) {
	f, err := os.Open(workloadPath)
	if err != nil {
		return nil, err
	}
	info, _ := f.Stat()

	ctx := context.Background()
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	f.Seek(0, 0)

	stream, err := c.client.SubmitWorkload(ctx)
	if err != nil {
		return nil, err
	}
	err = stream.Send(&Workload{
		ChunkType: &Workload_Header{
			Header: &WorkloadMetadata{
				Hash:               fmt.Sprintf("%x", h.Sum(nil)),
				RuntimeEnvironment: env,
				Name:               workloadPath,
				TotalBytes:         int32(info.Size()),
			},
		},
	})

	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	buf := make([]byte, 1024*16)
	for {
		n, err := f.Read(buf)
		if err != nil {
			if err == io.EOF {
				fmt.Println("EOF")
				break
			}
			fmt.Println(err)
			return nil, err
		}

		if err := stream.Send(&Workload{
			ChunkType: &Workload_Chunk{
				Chunk: buf[:n],
			},
		}); err != nil {
			if err == io.EOF {
				break
			}
			fmt.Println(err)
			return nil, err
		}
	}

	ack, err := stream.CloseAndRecv()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	return &WorkloadAck{Error: ack.Error, Message: ack.Message}, nil
}

func (c AgentClient) WaitForAgentToBoot(ctx context.Context) error {
	time.Sleep(75 * time.Millisecond)
	// Query the agent until it provides a valid response
	req.SetTimeout(500 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			// Timeout
			return ctx.Err()
		default:
			healthy, version, err := c.Health()
			if err != nil {
				log.WithError(err).Info("Nex agent not ready yet (timeout)")
				time.Sleep(time.Second)
				continue
			}

			if !healthy {
				time.Sleep(time.Second)
				log.Info("Nex agent running but indicates not yet ready")
			} else {
				log.WithField("ip", c.ip).WithField("version", version).Info("Nex agent ready")
				return nil
			}
			time.Sleep(time.Second)
		}
	}
}
