package client

import (
	"context"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/synadia-labs/nex/_test"
	"github.com/synadia-labs/nex/models"
)

func TestNewNexClient(t *testing.T) {
	server := _test.StartNatsServer(t, t.TempDir())
	defer func() {
		for server.NumClients() == 0 {
			server.Shutdown()
			return
		}
	}()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	client, err := NewClient(context.Background(), nc, "test")
	be.NilErr(t, err)
	be.Nonzero(t, client)

	be.DeepEqual(t, nc, client.nc)
	be.Equal(t, "test", client.namespace)
}

func TestNewNexClientWithOptions(t *testing.T) {
	server := _test.StartNatsServer(t, t.TempDir())
	defer func() {
		for server.NumClients() == 0 {
			server.Shutdown()
			return
		}
	}()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	customTimeout := 30 * time.Second
	customStall := 5 * time.Second
	customStartTimeout := 2 * time.Minute

	client, err := NewClient(context.Background(), nc, "test",
		WithDefaultTimeout(customTimeout),
		WithStartWorkloadTimeout(customStartTimeout),
		WithRequestManyStall(customStall),
	)
	be.NilErr(t, err)
	be.Nonzero(t, client)

	be.Equal(t, customTimeout, client.defaultTimeout)
	be.Equal(t, customStartTimeout, client.startWorkloadTimeout)
	be.Equal(t, customStall, client.requestManyStall)
}

func TestNexClient_User(t *testing.T) {
	workDir := t.TempDir()
	server := _test.StartNatsServer(t, workDir)
	defer func() {
		for server.NumClients() == 0 {
			server.Shutdown()
			return
		}
	}()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), 1, false)
	be.Equal(t, 1, len(nexNodes))

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	client, err := NewClient(context.Background(), nc, "user")
	be.NilErr(t, err)
	be.Nonzero(t, client)

	ar, err := client.Auction("inmem", map[string]string{})
	be.NilErr(t, err)
	be.Equal(t, 1, len(ar))

	sr, err := client.StartWorkload(ar[0].BidderId, "tester", "My test workload", "{}", "inmem", models.WorkloadLifecycleService, nil)
	be.NilErr(t, err)
	be.Equal(t, "tester", sr.Name)

	str, err := client.StopWorkload(sr.Id)
	be.NilErr(t, err)

	be.Equal(t, sr.Id, str.Id)
	be.True(t, str.Stopped)

	for _, node := range nexNodes {
		be.NilErr(t, node.Shutdown())
	}
}

func TestNexClient_System(t *testing.T) {
	nodeSize := []struct {
		name string
		size int
	}{
		{"OneNodeNexus", 1},
		{"ThreeNodeNexus", 3},
		{"FiveNodeNexus", 5},
	}

	for _, tt := range nodeSize {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			server := _test.StartNatsServer(t, workDir)
			defer func() {
				for server.NumClients() == 0 {
					server.Shutdown()
					return
				}
			}()

			nc, err := nats.Connect(server.ClientURL())
			be.NilErr(t, err)
			defer nc.Close()

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), tt.size, false)
			be.Equal(t, tt.size, len(nexNodes))

			nc, err = nats.Connect(server.ClientURL())
			be.NilErr(t, err)
			defer nc.Close()

			client, err := NewClient(context.Background(), nc, models.SystemNamespace)
			be.NilErr(t, err)
			be.Nonzero(t, client)

			info, err := client.GetNodeInfo(_test.Node1Pub)
			be.NilErr(t, err)
			be.Equal(t, _test.Node1Pub, info.NodeId)

			nodes, err := client.ListNodes(map[string]string{"nex.node": "testnexus-1"})
			be.NilErr(t, err)
			be.Equal(t, 1, len(nodes))
			be.Equal(t, _test.Node1Pub, nodes[0].NodeId)

			ldr, err := client.SetLameduck(_test.Node1Pub, 0, nil)
			be.NilErr(t, err)
			be.True(t, ldr.Success)
			time.Sleep(250 * time.Millisecond)

			nodes, err = client.ListNodes(nil)
			be.NilErr(t, err)
			be.Equal(t, tt.size-1, len(nodes))

			for _, node := range nexNodes {
				be.NilErr(t, node.Shutdown())
			}
		})
	}
}

func TestNexClient_SystemAsUser(t *testing.T) {
	workDir := t.TempDir()
	server := _test.StartNatsServer(t, workDir)
	defer func() {
		for server.NumClients() == 0 {
			server.Shutdown()
			return
		}
	}()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), 1, false)
	be.Equal(t, 1, len(nexNodes))

	client, err := NewClient(context.Background(), nc, "user")
	be.NilErr(t, err)
	be.Nonzero(t, client)

	_, err = client.GetNodeInfo(_test.Node1Pub)
	be.Equal(t, "node not found", err.Error())

	nodes, err := client.ListNodes(map[string]string{"foo": "bar"})
	be.NilErr(t, err)
	//	be.Equal(t, "no nodes found", err.Error())
	be.Equal(t, 0, len(nodes))

	ldresp, err := client.SetLameduck(_test.Node1Pub, 0, nil)
	be.NilErr(t, err)
	be.False(t, ldresp.Success)

	for _, node := range nexNodes {
		be.NilErr(t, node.Shutdown())
	}
}

func TestNexClient_ListWorkloads(t *testing.T) {
	nodeSize := []struct {
		name string
		size int
	}{
		{"OneNodeNexus", 1},
		{"ThreeNodeNexus", 3},
		{"FiveNodeNexus", 5},
	}

	for _, tt := range nodeSize {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			server := _test.StartNatsServer(t, workDir)
			defer func() {
				for server.NumClients() == 0 {
					server.Shutdown()
					return
				}
			}()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), 1, false)
			be.Equal(t, 1, len(nexNodes))

			nc, err := nats.Connect(server.ClientURL())
			be.NilErr(t, err)
			defer nc.Close()

			client, err := NewClient(context.Background(), nc, "user")
			be.NilErr(t, err)
			be.Nonzero(t, client)

			ar, err := client.Auction("inmem", map[string]string{})
			be.NilErr(t, err)
			be.Equal(t, 1, len(ar))

			_, err = client.StartWorkload(ar[0].BidderId, "tester1", "My test workload", "{}", "inmem", models.WorkloadLifecycleService, nil)
			be.NilErr(t, err)

			ar, err = client.Auction("inmem", map[string]string{})
			be.NilErr(t, err)
			be.Equal(t, 1, len(ar))

			_, err = client.StartWorkload(ar[0].BidderId, "tester2", "My test workload", "{}", "inmem", models.WorkloadLifecycleService, nil)
			be.NilErr(t, err)

			ar, err = client.Auction("inmem", map[string]string{})
			be.NilErr(t, err)
			be.Equal(t, 1, len(ar))

			_, err = client.StartWorkload(ar[0].BidderId, "tester3", "My test workload", "{}", "inmem", models.WorkloadLifecycleService, nil)
			be.NilErr(t, err)

			wl, err := client.ListWorkloads([]string{})
			be.NilErr(t, err)

			be.Equal(t, 1, len(wl))
			be.Equal(t, 3, len(*wl[0]))

			for _, node := range nexNodes {
				be.NilErr(t, node.Shutdown())
			}
		})
	}
}

func TestNexClient_List_NoNodes(t *testing.T) {
	server := _test.StartNatsServer(t, t.TempDir())
	defer func() {
		for server.NumClients() == 0 {
			server.Shutdown()
		}
	}()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	tt := []string{
		models.SystemNamespace,
		"user",
	}

	for _, ns := range tt {
		t.Run("As:"+ns, func(t *testing.T) {
			client, err := NewClient(context.Background(), nc, models.SystemNamespace)
			be.NilErr(t, err)
			be.Nonzero(t, client)

			t.Run("ListNodes", func(t *testing.T) {
				nodes, err := client.ListNodes(nil)
				be.NilErr(t, err)
				be.Equal(t, 0, len(nodes))
			})

			t.Run("ListWorkloads", func(t *testing.T) {
				workloads, err := client.ListWorkloads(nil)
				be.NilErr(t, err)
				be.Equal(t, 0, len(workloads))
			})
		})
	}
}

func TestNexClient_CloneWorkload(t *testing.T) {
	nodeSize := []struct {
		name string
		size int
	}{
		{"OneNodeNexus", 1},
		{"ThreeNodeNexus", 3},
		{"FiveNodeNexus", 5},
	}

	for _, tt := range nodeSize {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			server := _test.StartNatsServer(t, workDir)
			defer func() {
				for server.NumClients() == 0 {
					server.Shutdown()
					return
				}
			}()

			nc, err := nats.Connect(server.ClientURL())
			be.NilErr(t, err)
			defer nc.Close()

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), tt.size, false)
			be.Equal(t, tt.size, len(nexNodes))

			nc, err = nats.Connect(server.ClientURL())
			be.NilErr(t, err)
			defer nc.Close()

			client, err := NewClient(context.Background(), nc, "user")
			be.NilErr(t, err)
			be.Nonzero(t, client)

			ar, err := client.Auction("inmem", map[string]string{})
			be.NilErr(t, err)
			be.Equal(t, tt.size, len(ar))

			swr, err := client.StartWorkload(ar[0].BidderId, "tester1", "My test workload", "{}", "inmem", models.WorkloadLifecycleService, nil)
			be.NilErr(t, err)

			_, err = client.CloneWorkload(swr.Id, nil)
			be.NilErr(t, err)

			time.Sleep(250 * time.Millisecond)

			wl, err := client.ListWorkloads(nil)
			be.NilErr(t, err)

			totalCount := 0
			for _, w := range wl {
				totalCount += len(*w)
			}

			be.Equal(t, 2, totalCount)

			for _, node := range nexNodes {
				be.NilErr(t, node.Shutdown())
			}
		})
	}
}

func TestNexClient_StopWorkloadDNE(t *testing.T) {
	nodeSize := []struct {
		name string
		size int
	}{
		{"OneNodeNexus", 1},
		{"ThreeNodeNexus", 3},
		{"FiveNodeNexus", 5},
	}

	for _, tt := range nodeSize {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			server := _test.StartNatsServer(t, workDir)
			defer func() {
				for server.NumClients() == 0 {
					server.Shutdown()
					return
				}
			}()

			nc, err := nats.Connect(server.ClientURL())
			be.NilErr(t, err)
			defer nc.Close()

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), tt.size, false)
			be.Equal(t, tt.size, len(nexNodes))

			nc, err = nats.Connect(server.ClientURL())
			be.NilErr(t, err)
			defer nc.Close()

			client, err := NewClient(context.Background(), nc, "user")
			be.NilErr(t, err)
			be.Nonzero(t, client)

			str, err := client.StopWorkload("abc123")
			be.NilErr(t, err)

			be.False(t, str.Stopped)
			be.Equal(t, "", str.WorkloadType)
			be.Equal(t, "abc123", str.Id)
			be.Equal(t, string(models.GenericErrorsWorkloadNotFound), str.Message)

			for _, node := range nexNodes {
				be.NilErr(t, node.Shutdown())
			}
		})
	}
}

func TestNexClient_StopWorkload(t *testing.T) {
	nodeSize := []struct {
		name string
		size int
	}{
		{"OneNodeNexus", 1},
		{"ThreeNodeNexus", 3},
		{"FiveNodeNexus", 5},
	}

	for _, tt := range nodeSize {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			server := _test.StartNatsServer(t, workDir)
			defer func() {
				for server.NumClients() == 0 {
					server.Shutdown()
					return
				}
			}()

			nc, err := nats.Connect(server.ClientURL())
			be.NilErr(t, err)
			defer nc.Close()

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), tt.size, false)
			be.Equal(t, tt.size, len(nexNodes))

			nc, err = nats.Connect(server.ClientURL())
			be.NilErr(t, err)
			defer nc.Close()

			client, err := NewClient(context.Background(), nc, "user")
			be.NilErr(t, err)
			be.Nonzero(t, client)

			ar, err := client.Auction("inmem", map[string]string{})
			be.NilErr(t, err)
			be.Equal(t, tt.size, len(ar))

			swr, err := client.StartWorkload(ar[0].BidderId, "tester1", "My test workload", "{}", "inmem", models.WorkloadLifecycleService, nil)
			be.NilErr(t, err)

			time.Sleep(250 * time.Millisecond)

			str, err := client.StopWorkload(swr.Id)
			be.NilErr(t, err)

			be.True(t, str.Stopped)
			be.Equal(t, "inmem", str.WorkloadType)
			be.Equal(t, swr.Id, str.Id)
			be.Zero(t, str.Message)

			for _, node := range nexNodes {
				be.NilErr(t, node.Shutdown())
			}
		})
	}
}
