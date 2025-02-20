package nex

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/synadia-labs/nex/internal"
	"github.com/synadia-labs/nex/models"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	sdk "github.com/synadia-io/nexlet.go/agent"
)

func WithNodeName(name string) NexNodeOption {
	return func(n *NexNode) error {
		n.name = name
		return nil
	}
}

func WithNexus(nexus string) NexNodeOption {
	return func(n *NexNode) error {
		n.nexus = nexus
		return nil
	}
}

func WithContext(ctx context.Context) NexNodeOption {
	return func(n *NexNode) error {
		n.ctx = ctx
		return nil
	}
}

func WithLogger(logger *slog.Logger) NexNodeOption {
	return func(n *NexNode) error {
		n.logger = logger
		return nil
	}
}

func WithNatsConn(nc *nats.Conn) NexNodeOption {
	return func(n *NexNode) error {
		if nc != nil {
			n.nc = nc
		}
		return nil
	}
}

func WithInternalNatsServer(opts *server.Options) NexNodeOption {
	return func(n *NexNode) error {
		var err error
		n.server, err = server.NewServer(opts)
		return err
	}
}

func WithNodeKeyPair(kp nkeys.KeyPair) NexNodeOption {
	return func(n *NexNode) error {
		n.nodeKeypair = kp
		return nil
	}
}

func WithNodeXKeyPair(kp nkeys.KeyPair) NexNodeOption {
	return func(n *NexNode) error {
		n.nodeXKeypair = kp
		return nil
	}
}

func WithAgentRunner(agent *sdk.Runner) NexNodeOption {
	return func(n *NexNode) error {
		n.embeddedRunners = append(n.embeddedRunners, agent)
		return nil
	}
}

func WithLocalAgent(agent models.Agent) NexNodeOption {
	return func(n *NexNode) error {
		ap := &internal.AgentProcess{
			Config: &agent,
		}
		n.localRunners = append(n.localRunners, ap)
		return nil
	}
}

func WithTag(key, value string) NexNodeOption {
	return func(n *NexNode) error {
		for _, prefix := range models.ReservedTagPrefixes {
			if strings.HasPrefix(key, prefix) {
				return errors.New("can not use reserved tag prefix: " + prefix)
			}
		}
		n.tags[key] = value
		return nil
	}
}

func WithAllowAgentRegistration() NexNodeOption {
	return func(n *NexNode) error {
		n.allowAgentRegistration = true
		return nil
	}
}

func WithState(s models.NexNodeState) NexNodeOption {
	return func(n *NexNode) error {
		n.state = s
		return nil
	}
}

func WithSigningKey(seed, issuer string) NexNodeOption {
	return func(n *NexNode) error {
		n.signingKey = seed
		n.issuerAcct = issuer
		return nil
	}
}

func WithMinter(m models.CredVendor) NexNodeOption {
	return func(n *NexNode) error {
		n.minter = m
		return nil
	}
}
