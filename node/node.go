package node

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go"
)

type Node interface {
	Validate() error
	Start()
}

type nexNode struct {
	nc *nats.Conn

	logger *slog.Logger
}

type NexOption func(*nexNode)

func NewNexNode(nc *nats.Conn, opts ...NexOption) (Node, error) {
	if nc == nil {
		return nil, fmt.Errorf("no nats connection provided")
	}

	nn := &nexNode{
		nc:     nc,
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})),
	}

	for _, opt := range opts {
		opt(nn)
	}

	return nn, nil
}

func (nn *nexNode) Validate() error {
	var errs error

	return errs
}

func (nn *nexNode) Start() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	<-ctx.Done()
}
