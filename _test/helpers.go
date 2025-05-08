package _test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nexlet.go/agent"
	"github.com/synadia-labs/nex"
	testminter "github.com/synadia-labs/nex/_test/minter"
	inmem "github.com/synadia-labs/nex/_test/nexlet_inmem"
)

func init() {
	_, found := os.LookupEnv("NEX_TEST_DEBUG")
	if found {
		fmt.Println("*** USING DEBUG MODE | ALL ***")
		debugModeNatsDefault = true
		debugNodeNoRunner = true
		return
	}
	_, debugModeNatsDefault = os.LookupEnv("NEX_TEST_DEBUG_NATS")
	if debugModeNatsDefault {
		fmt.Println("*** USING DEBUG MODE | NATS ***")
	}
	_, debugNodeNoRunner = os.LookupEnv("NEX_TEST_DEBUG_NODE_RUNNER")
	if debugNodeNoRunner {
		fmt.Println("*** USING DEBUG MODE | NODE RUNNER ***")
	}
}

const (
	Node1Seed = "SNAD2VTZQ7Z3CCB6TJWFFJK6J7DDZ5SQUHOFPQFRKDIVDQBM2WLGZGELSE"
	Node1Pub  = "NB4XOM2IOV2NRLRZZU5PFMQFBAQXYQOG4WOVGOWRYJRBWI3JO5QJK7AG"
)

var (
	debugModeNatsDefault bool
	debugNodeNoRunner    bool
)

func StartNatsServer(t testing.TB, workDir string) *server.Server {
	t.Helper()

	server := server.New(&server.Options{
		Port: func() int {
			if debugModeNatsDefault {
				return 4222
			}
			return -1
		}(),
		JetStream: true,
		StoreDir:  workDir,
	})

	server.Start()

	return server
}

func StartNexus(t testing.TB, ctx context.Context, natsUrl string, size int, state bool, runners ...*agent.Runner) []*nex.NexNode {
	t.Helper()

	showTestLogsEnv, err := strconv.ParseBool(os.Getenv("NEX_TEST_LOGS"))
	if err != nil {
		showTestLogsEnv = false
	}

	ret := make([]*nex.NexNode, size)
	for i := 0; i < size; i++ {
		logger := slog.New(slog.NewTextHandler(func() io.Writer {
			if showTestLogsEnv {
				return os.Stdout
			}
			return io.Discard
		}(), &slog.HandlerOptions{Level: slog.LevelDebug}))

		nc, err := nats.Connect(natsUrl)
		be.NilErr(t, err)

		var kp nkeys.KeyPair
		if i == 0 {
			kp, err = nkeys.FromSeed([]byte(Node1Seed))
			be.NilErr(t, err)
		} else {
			kp, err = nkeys.CreateServer()
			be.NilErr(t, err)
		}

		pubKey, err := kp.PublicKey()
		be.NilErr(t, err)

		minter := testminter.TestMinter{NatsServer: nc.ConnectedUrl()}

		opts := []nex.NexNodeOption{
			nex.WithNodeName(fmt.Sprintf("testnexus-%d", i+1)),
			nex.WithNexus("testnexus"),
			nex.WithNodeKeyPair(kp),
			nex.WithNatsConn(nc),
			nex.WithMinter(&minter),
			nex.WithLogger(logger),
			nex.WithTag("foo", "bar"),
		}

		for _, runner := range runners {
			opts = append(opts, nex.WithAgentRunner(runner))
		}

		if len(runners) == 0 {
			runner, err := inmem.NewInMemAgent(pubKey, logger)
			be.NilErr(t, err)
			opts = append(opts, nex.WithAgentRunner(runner))
		}

		node, err := nex.NewNexNode(opts...)
		be.NilErr(t, err)
		be.NilErr(t, node.Start())

		for node.IsReady() {
			break
		}

		ret[i] = node
	}
	return ret
}
