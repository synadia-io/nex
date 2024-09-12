package node

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go"
)

type NexNode struct {
	nc *nats.Conn

	logger *slog.Logger
}

type NexOption func(*NexNode)

func NewNexNode(nc *nats.Conn, opts ...NexOption) (*NexNode, error) {
	if nc == nil {
		return nil, fmt.Errorf("no nats connection provided")
	}

	nexNode := &NexNode{
		nc:     nc,
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})),
	}

	for _, opt := range opts {
		opt(nexNode)
	}

	return nexNode, nil
}

func (nn NexNode) Validate() error {
	var errs error

	return errs
}

func (NexNode) Start() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	<-ctx.Done()
}
