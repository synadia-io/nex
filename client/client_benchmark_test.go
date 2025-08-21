package client

import (
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/synadia-labs/nex/_test"
)

func BenchmarkClientAuction(b *testing.B) {
	const (
		namespace    = "user"
		workloadType = "inmem"
	)

	testCases := []struct {
		name        string
		clusterSize int
		tags        map[string]string
		stallTime   time.Duration
	}{
		{
			"1NodeQuickStall",
			1,
			map[string]string{},
			100 * time.Millisecond,
		},
		{
			"3NodesQuickStall",
			3,
			map[string]string{},
			100 * time.Millisecond,
		},
		{
			"5NodesQuickStall",
			5,
			map[string]string{},
			100 * time.Millisecond,
		},
		{
			"1NodeLongStall",
			1,
			map[string]string{},
			1 * time.Second,
		},
		{
			"3NodesLongStall",
			3,
			map[string]string{},
			1 * time.Second,
		},
		{
			"5NodesLongStall",
			5,
			map[string]string{},
			1 * time.Second,
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {

			workDir := b.TempDir()
			server := _test.StartNatsServer(b, workDir)
			defer server.Shutdown()

			nc, err := nats.Connect(server.ClientURL())
			be.NilErr(b, err)

			ctx := b.Context()

			nexNodes := _test.StartNexus(b, ctx, server.ClientURL(), tc.clusterSize, false)
			defer func() {
				for _, nn := range nexNodes {
					nn.Shutdown()
				}
			}()
			nc.Close()

			// open a new fresh connection for the nex client
			nc, err = nats.Connect(server.ClientURL())
			be.NilErr(b, err)
			defer nc.Close()

			client, err := NewClient(b.Context(), nc, namespace, WithAuctionStall(tc.stallTime))
			be.NilErr(b, err)
			be.Nonzero(b, client)

			b.ResetTimer()
			for b.Loop() {
				auctionResponses, err := client.Auction(workloadType, map[string]string{})
				be.NilErr(b, err)
				b.StopTimer()
				for _, ar := range auctionResponses {
					be.Nonzero(b, ar)
				}
				b.StartTimer()
			}
		})
	}

	b.ReportAllocs() // Report memory allocations
}
