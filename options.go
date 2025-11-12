package nex

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/synadia-io/nex/internal"
	"github.com/synadia-io/nex/models"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	sdk "github.com/synadia-io/nex/sdk/go/agent"
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

func WithInternalNatsServer(opts *server.Options, creds *models.NatsConnectionData) NexNodeOption {
	return func(n *NexNode) error {
		n.serverCreds = creds
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

func WithAgentRestartLimit(limit int) NexNodeOption {
	return func(n *NexNode) error {
		if limit < 0 {
			return errors.New("agent restart limit must be non-negative")
		}
		n.agentRestartLimit = limit
		return nil
	}
}

func WithAgentRunner(agent *sdk.Runner) NexNodeOption {
	return func(n *NexNode) error {
		n.embeddedRunners = append(n.embeddedRunners, agent)
		return nil
	}
}

func WithAgent(agent models.Agent) NexNodeOption {
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

func WithAllowRemoteAgentRegistration() NexNodeOption {
	return func(n *NexNode) error {
		n.allowRemoteAgentRegistration = true
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

func WithAuctioneer(a models.Auctioneer) NexNodeOption {
	return func(n *NexNode) error {
		n.auctioneer = a
		return nil
	}
}

func WithIDGenerator(a models.IDGen) NexNodeOption {
	return func(n *NexNode) error {
		n.idgen = a
		return nil
	}
}

func WithAgentRegistrar(a models.AgentRegistrar) NexNodeOption {
	return func(n *NexNode) error {
		n.aregistrar = a
		return nil
	}
}

func WithSecretStore(s models.SecretStore) NexNodeOption {
	return func(n *NexNode) error {
		n.secretStore = s
		return nil
	}
}

func WithEventEmitter(e models.EventEmitter) NexNodeOption {
	return func(n *NexNode) error {
		n.eventEmitter = e
		return nil
	}
}
