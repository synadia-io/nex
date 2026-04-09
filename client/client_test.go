package client

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/synadia-io/nex/_test"
	"github.com/synadia-io/nex/models"
)

func TestNewNexClient(t *testing.T) {
	server := _test.StartNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	client, err := NewClient(context.Background(), nc, "test")
	be.NilErr(t, err)
	be.Nonzero(t, client)

	c := client.(*nexClient)
	be.DeepEqual(t, nc, c.nc)
	be.Equal(t, "test", c.namespace)
}

func TestNewNexClientWithOptions(t *testing.T) {
	server := _test.StartNatsServer(t, t.TempDir())
	defer server.Shutdown()

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

	c := client.(*nexClient)
	be.Equal(t, customTimeout, c.defaultTimeout)
	be.Equal(t, customStartTimeout, c.startWorkloadTimeout)
	be.Equal(t, customStall, c.requestManyStall)
}

func TestNexClient_User(t *testing.T) {
	workDir := t.TempDir()
	server := _test.StartNatsServer(t, workDir)
	defer server.Shutdown()

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

	var ar []*models.AuctionResponse
	_test.WaitFor(t, 10*time.Second, func() bool {
		ar, err = client.Auction("user", "inmem", map[string]string{})
		return err == nil && len(ar) == 1
	}, "waiting for auction to return 1 result")

	sr, err := client.StartWorkload(ar[0].BidderId, &models.StartWorkloadRequest{
		Namespace:         "user",
		Name:              "tester",
		Description:       "My test workload",
		RunRequest:        "{}",
		WorkloadType:      "inmem",
		WorkloadLifecycle: models.WorkloadLifecycleService,
	})
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
			defer server.Shutdown()

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

			_test.WaitFor(t, 10*time.Second, func() bool {
				nodes, err = client.ListNodes(nil)
				return err == nil && len(nodes) == tt.size-1
			}, "waiting for lameduck node to disappear from list")
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
	defer server.Shutdown()

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
			defer server.Shutdown()

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), tt.size, false)
			be.Equal(t, tt.size, len(nexNodes))

			nc, err := nats.Connect(server.ClientURL())
			be.NilErr(t, err)
			defer nc.Close()

			client, err := NewClient(context.Background(), nc, "user")
			be.NilErr(t, err)
			be.Nonzero(t, client)

			var ar []*models.AuctionResponse
			_test.WaitFor(t, 10*time.Second, func() bool {
				ar, err = client.Auction("user", "inmem", map[string]string{})
				return err == nil && len(ar) == tt.size
			}, "waiting for auction to return expected results")

			_, err = client.StartWorkload(ar[0].BidderId, &models.StartWorkloadRequest{
				Namespace:         "user",
				Name:              "tester1",
				Description:       "My test workload",
				RunRequest:        "{}",
				WorkloadType:      "inmem",
				WorkloadLifecycle: models.WorkloadLifecycleService,
			})
			be.NilErr(t, err)

			_test.WaitFor(t, 10*time.Second, func() bool {
				ar, err = client.Auction("user", "inmem", map[string]string{})
				return err == nil && len(ar) == tt.size
			}, "waiting for auction to return expected results")

			_, err = client.StartWorkload(ar[1%tt.size].BidderId, &models.StartWorkloadRequest{
				Namespace:         "user",
				Name:              "tester2",
				Description:       "My test workload",
				RunRequest:        "{}",
				WorkloadType:      "inmem",
				WorkloadLifecycle: models.WorkloadLifecycleService,
			})
			be.NilErr(t, err)

			_test.WaitFor(t, 10*time.Second, func() bool {
				ar, err = client.Auction("user", "inmem", map[string]string{})
				return err == nil && len(ar) == tt.size
			}, "waiting for auction to return expected results")

			_, err = client.StartWorkload(ar[2%tt.size].BidderId, &models.StartWorkloadRequest{
				Namespace:         "user",
				Name:              "tester3",
				Description:       "My test workload",
				RunRequest:        "{}",
				WorkloadType:      "inmem",
				WorkloadLifecycle: models.WorkloadLifecycleService,
			})
			be.NilErr(t, err)

			wl, err := client.ListWorkloads([]string{})
			be.NilErr(t, err)

			totalCount := 0
			for _, w := range wl {
				totalCount += len(*w)
			}
			be.Equal(t, 3, totalCount)

			for _, node := range nexNodes {
				be.NilErr(t, node.Shutdown())
			}
		})
	}
}

func TestNexClient_List_NoNodes(t *testing.T) {
	server := _test.StartNatsServer(t, t.TempDir())
	defer server.Shutdown()

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
			defer server.Shutdown()

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

			client, err := NewClient(context.Background(), nc, "user", WithAuctionStall(5*time.Second))
			be.NilErr(t, err)
			be.Nonzero(t, client)

			var ar []*models.AuctionResponse
			_test.WaitFor(t, 10*time.Second, func() bool {
				ar, err = client.Auction("user", "inmem", map[string]string{})
				return err == nil && len(ar) == tt.size
			}, "waiting for auction to return expected results")

			swr, err := client.StartWorkload(ar[0].BidderId, &models.StartWorkloadRequest{
				Namespace:         "user",
				Name:              "tester1",
				Description:       "My test workload",
				RunRequest:        "{}",
				WorkloadType:      "inmem",
				WorkloadLifecycle: models.WorkloadLifecycleService,
			})
			be.NilErr(t, err)

			_, err = client.CloneWorkload(swr.Id, nil)
			be.NilErr(t, err)

			var totalCount int
			_test.WaitFor(t, 10*time.Second, func() bool {
				wl, wlErr := client.ListWorkloads(nil)
				if wlErr != nil {
					return false
				}
				totalCount = 0
				for _, w := range wl {
					totalCount += len(*w)
				}
				return totalCount == 2
			}, "waiting for cloned workload to appear")
			be.Equal(t, 2, totalCount)

			for _, node := range nexNodes {
				be.NilErr(t, node.Shutdown())
			}
		})
	}
}

func TestNexClient_GetNexusPTags(t *testing.T) {
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
			defer server.Shutdown()

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

			tags, err := client.GetNexusPTags()
			be.NilErr(t, err)
			be.Equal(t, "bar", tags["foo"])

			for _, node := range nexNodes {
				be.NilErr(t, node.Shutdown())
			}
		})
	}
}

func TestNexClient_GetNexusPTags_NoNodes(t *testing.T) {
	server := _test.StartNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	client, err := NewClient(context.Background(), nc, models.SystemNamespace)
	be.NilErr(t, err)
	be.Nonzero(t, client)

	tags, err := client.GetNexusPTags()
	be.NilErr(t, err)
	be.Equal(t, 0, len(tags))
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
			defer server.Shutdown()

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
			defer server.Shutdown()

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

			var ar []*models.AuctionResponse
			_test.WaitFor(t, 10*time.Second, func() bool {
				ar, err = client.Auction("user", "inmem", map[string]string{})
				return err == nil && len(ar) == tt.size
			}, "waiting for auction to return expected results")

			swr, err := client.StartWorkload(ar[0].BidderId, &models.StartWorkloadRequest{
				Namespace:         "user",
				Name:              "tester1",
				Description:       "My test workload",
				RunRequest:        "{}",
				WorkloadType:      "inmem",
				WorkloadLifecycle: models.WorkloadLifecycleService,
			})
			be.NilErr(t, err)

			_test.WaitFor(t, 10*time.Second, func() bool {
				wl, wlErr := client.ListWorkloads(nil)
				if wlErr != nil {
					return false
				}
				totalCount := 0
				for _, w := range wl {
					totalCount += len(*w)
				}
				return totalCount == 1
			}, "waiting for workload to be running")

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

// fakeErrorResponder subscribes to the given subject and simulates a nex node that returns an error.
func fakeErrorResponder(t *testing.T, nc *nats.Conn, subject string, code int, msg string) *nats.Subscription {
	t.Helper()

	errBody, _ := json.Marshal(struct {
		ErrorID string `json:"error_id"`
		Error   string `json:"error"`
	}{
		ErrorID: "fake-error-node",
		Error:   msg,
	})

	sub, err := nc.Subscribe(subject, func(m *nats.Msg) {
		resp := nats.NewMsg(m.Reply)
		resp.Header.Set(micro.ErrorCodeHeader, strconv.Itoa(code))
		resp.Header.Set(micro.ErrorHeader, msg)
		resp.Data = errBody
		_ = nc.PublishMsg(resp)
	})
	be.NilErr(t, err)
	return sub
}

func TestNexClient_ListNodes_PartialError(t *testing.T) {
	workDir := t.TempDir()
	server := _test.StartNatsServer(t, workDir)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), 3, false)
	be.Equal(t, 3, len(nexNodes))

	errNC, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer errNC.Close()

	sub := fakeErrorResponder(t, errNC, models.PingRequestSubject(models.SystemNamespace), 500, "internal node error")
	defer func() { _ = sub.Unsubscribe() }()

	nc, err = nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	client, err := NewClient(context.Background(), nc, models.SystemNamespace)
	be.NilErr(t, err)
	be.Nonzero(t, client)

	var nodes []*models.NodePingResponse
	_test.WaitFor(t, 10*time.Second, func() bool {
		nodes, err = client.ListNodes(nil)
		return len(nodes) == 3
	}, "waiting for list nodes to return 3 healthy results")

	be.Nonzero(t, err)
	be.Equal(t, 3, len(nodes))

	for _, node := range nexNodes {
		be.NilErr(t, node.Shutdown())
	}
}

func TestNexClient_ListWorkloads_PartialError(t *testing.T) {
	workDir := t.TempDir()
	server := _test.StartNatsServer(t, workDir)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), 3, false)
	be.Equal(t, 3, len(nexNodes))

	nc, err = nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	client, err := NewClient(context.Background(), nc, "user")
	be.NilErr(t, err)
	be.Nonzero(t, client)

	var ar []*models.AuctionResponse
	_test.WaitFor(t, 10*time.Second, func() bool {
		ar, err = client.Auction("user", "inmem", map[string]string{})
		return err == nil && len(ar) == 3
	}, "waiting for auction to return 3 results")

	_, err = client.StartWorkload(ar[0].BidderId, &models.StartWorkloadRequest{
		Namespace:         "user",
		Name:              "tester1",
		Description:       "My test workload",
		RunRequest:        "{}",
		WorkloadType:      "inmem",
		WorkloadLifecycle: models.WorkloadLifecycleService,
	})
	be.NilErr(t, err)

	_test.WaitFor(t, 10*time.Second, func() bool {
		wl, wlErr := client.ListWorkloads(nil)
		if wlErr != nil {
			return false
		}
		totalCount := 0
		for _, w := range wl {
			totalCount += len(*w)
		}
		return totalCount == 1
	}, "waiting for workload to be running")

	errNC, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer errNC.Close()

	sub := fakeErrorResponder(t, errNC, models.NamespacePingRequestSubject("user"), 500, "node unavailable")
	defer func() { _ = sub.Unsubscribe() }()
	be.NilErr(t, errNC.Flush())

	wl, err := client.ListWorkloads([]string{})
	be.Nonzero(t, err)

	totalCount := 0
	for _, w := range wl {
		totalCount += len(*w)
	}
	be.Equal(t, 1, totalCount)

	for _, node := range nexNodes {
		be.NilErr(t, node.Shutdown())
	}
}

func TestNexClient_Auction_PartialError(t *testing.T) {
	workDir := t.TempDir()
	server := _test.StartNatsServer(t, workDir)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), 3, false)
	be.Equal(t, 3, len(nexNodes))

	errNC, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer errNC.Close()

	sub := fakeErrorResponder(t, errNC, models.AuctionRequestSubject("user"), 500, "auction failed")
	defer func() { _ = sub.Unsubscribe() }()

	nc, err = nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	client, err := NewClient(context.Background(), nc, "user")
	be.NilErr(t, err)
	be.Nonzero(t, client)

	var ar []*models.AuctionResponse
	_test.WaitFor(t, 10*time.Second, func() bool {
		ar, err = client.Auction("user", "inmem", map[string]string{})
		return len(ar) == 3
	}, "waiting for auction to return 3 healthy results")

	be.Nonzero(t, err)
	be.Equal(t, 3, len(ar))

	for _, node := range nexNodes {
		be.NilErr(t, node.Shutdown())
	}
}

// countWorkloads collapses the scatter-gather list response to a total count.
func countWorkloads(t *testing.T, c NexClient) int {
	t.Helper()
	wl, err := c.ListWorkloads(nil)
	be.NilErr(t, err)
	total := 0
	for _, w := range wl {
		total += len(*w)
	}
	return total
}

// TestNexClient_CloneWorkload_CrossNamespace verifies that when a system-user
// client clones a workload owned by a user namespace, the clone lands in the
// owning namespace — not in `system`. This was the primary bug motivating the
// cross-namespace fix: before the fix, client.CloneWorkload would pass through
// to StartWorkload with n.namespace="system" and the clone would be stored
// under n.workloads["system"] on the agent.
func TestNexClient_CloneWorkload_CrossNamespace(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	server := _test.StartNatsServer(t, workDir)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), 1, false)
	be.Equal(t, 1, len(nexNodes))

	// Build two clients sharing the same nexus: one scoped to the "user"
	// namespace that will deploy the original workload, and one scoped to
	// system that will perform the cross-namespace clone.
	userClient, err := NewClient(context.Background(), nc, "user", WithAuctionStall(5*time.Second))
	be.NilErr(t, err)

	systemClient, err := NewClient(context.Background(), nc, models.SystemNamespace, WithAuctionStall(5*time.Second))
	be.NilErr(t, err)

	var ar []*models.AuctionResponse
	_test.WaitFor(t, 10*time.Second, func() bool {
		ar, err = userClient.Auction("user", "inmem", map[string]string{})
		return err == nil && len(ar) == 1
	}, "waiting for user auction to return 1 result")

	origSwr, err := userClient.StartWorkload(ar[0].BidderId, &models.StartWorkloadRequest{
		Namespace:         "user",
		Name:              "orig",
		Description:       "original workload owned by user",
		RunRequest:        "{}",
		WorkloadType:      "inmem",
		WorkloadLifecycle: models.WorkloadLifecycleService,
	})
	be.NilErr(t, err)

	// Sanity: the user namespace now has exactly one workload, and a
	// system-scoped list sees the same single workload (system aggregates
	// across every namespace, and user is the only namespace with state).
	_test.WaitFor(t, 10*time.Second, func() bool {
		return countWorkloads(t, userClient) == 1
	}, "waiting for original to register in user namespace")
	be.Equal(t, 1, countWorkloads(t, userClient))
	be.Equal(t, 1, countWorkloads(t, systemClient))

	// System user clones the user-owned workload.
	cloneResp, err := systemClient.CloneWorkload(origSwr.Id, nil)
	be.NilErr(t, err)
	be.Nonzero(t, cloneResp)
	be.Unequal(t, origSwr.Id, cloneResp.Id)

	// Both the original and the clone must appear in the user namespace.
	// The system-namespace view aggregates every namespace, so it should
	// match the user view exactly (there is no namespace other than user).
	_test.WaitFor(t, 10*time.Second, func() bool {
		return countWorkloads(t, userClient) == 2
	}, "waiting for clone to appear in user namespace")
	be.Equal(t, 2, countWorkloads(t, userClient))

	// Verify each workload's Namespace field on the system list reports
	// "user", confirming the clone did not re-home into system.
	sysList, err := systemClient.ListWorkloads(nil)
	be.NilErr(t, err)
	seenUser := 0
	for _, agentResp := range sysList {
		for _, summary := range *agentResp {
			be.Nonzero(t, summary.Namespace)
			if summary.Namespace != nil && *summary.Namespace == "user" {
				seenUser++
			}
		}
	}
	be.Equal(t, 2, seenUser)

	for _, node := range nexNodes {
		be.NilErr(t, node.Shutdown())
	}
}

// TestNexClient_StopWorkload_System_Discovery verifies that a system-user
// client can stop a workload owned by a different namespace. The client must
// discover the owning namespace via ListWorkloads and publish the stop on
// that namespace's subject, not `system`.
func TestNexClient_StopWorkload_System_Discovery(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	server := _test.StartNatsServer(t, workDir)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), 1, false)
	be.Equal(t, 1, len(nexNodes))

	userClient, err := NewClient(context.Background(), nc, "user", WithAuctionStall(5*time.Second))
	be.NilErr(t, err)

	systemClient, err := NewClient(context.Background(), nc, models.SystemNamespace, WithAuctionStall(5*time.Second))
	be.NilErr(t, err)

	var ar []*models.AuctionResponse
	_test.WaitFor(t, 10*time.Second, func() bool {
		ar, err = userClient.Auction("user", "inmem", map[string]string{})
		return err == nil && len(ar) == 1
	}, "waiting for user auction to return 1 result")

	swr, err := userClient.StartWorkload(ar[0].BidderId, &models.StartWorkloadRequest{
		Namespace:         "user",
		Name:              "stoppable",
		Description:       "workload to be stopped by system",
		RunRequest:        "{}",
		WorkloadType:      "inmem",
		WorkloadLifecycle: models.WorkloadLifecycleService,
	})
	be.NilErr(t, err)

	_test.WaitFor(t, 10*time.Second, func() bool {
		return countWorkloads(t, userClient) == 1
	}, "waiting for workload to register")

	// System user stops the user-owned workload. The client must discover
	// namespace=user from the list response and target the user subject.
	stopResp, err := systemClient.StopWorkload(swr.Id)
	be.NilErr(t, err)
	be.True(t, stopResp.Stopped)
	be.Equal(t, swr.Id, stopResp.Id)

	// Confirm the workload is gone from the user namespace.
	_test.WaitFor(t, 10*time.Second, func() bool {
		return countWorkloads(t, userClient) == 0
	}, "waiting for workload to stop")

	for _, node := range nexNodes {
		be.NilErr(t, node.Shutdown())
	}
}

// TestNexClient_StopWorkload_System_NotFound verifies that a system-user stop
// for a nonexistent workload surfaces a clean not-found error rather than
// silently returning Stopped=false.
func TestNexClient_StopWorkload_System_NotFound(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	server := _test.StartNatsServer(t, workDir)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), 1, false)
	be.Equal(t, 1, len(nexNodes))

	systemClient, err := NewClient(context.Background(), nc, models.SystemNamespace, WithAuctionStall(5*time.Second))
	be.NilErr(t, err)

	_, err = systemClient.StopWorkload("does-not-exist")
	be.Nonzero(t, err)

	for _, node := range nexNodes {
		be.NilErr(t, node.Shutdown())
	}
}

// TestNexClient_CloneWorkload_StopOrig_CrossNamespace exercises the CLI's
// clone-then-stop flow as a system user against a user-owned workload. The
// clone must land in the user namespace and the original must be stopped.
func TestNexClient_CloneWorkload_StopOrig_CrossNamespace(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	server := _test.StartNatsServer(t, workDir)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	nexNodes := _test.StartNexus(t, ctx, server.ClientURL(), 1, false)
	be.Equal(t, 1, len(nexNodes))

	userClient, err := NewClient(context.Background(), nc, "user", WithAuctionStall(5*time.Second))
	be.NilErr(t, err)

	systemClient, err := NewClient(context.Background(), nc, models.SystemNamespace, WithAuctionStall(5*time.Second))
	be.NilErr(t, err)

	var ar []*models.AuctionResponse
	_test.WaitFor(t, 10*time.Second, func() bool {
		ar, err = userClient.Auction("user", "inmem", map[string]string{})
		return err == nil && len(ar) == 1
	}, "waiting for user auction to return 1 result")

	origSwr, err := userClient.StartWorkload(ar[0].BidderId, &models.StartWorkloadRequest{
		Namespace:         "user",
		Name:              "orig-stop",
		Description:       "original, will be stopped after clone",
		RunRequest:        "{}",
		WorkloadType:      "inmem",
		WorkloadLifecycle: models.WorkloadLifecycleService,
	})
	be.NilErr(t, err)

	_test.WaitFor(t, 10*time.Second, func() bool {
		return countWorkloads(t, userClient) == 1
	}, "waiting for original to register")

	// System-user clone then stop, mirroring `nex workload copy --stop`.
	cloneResp, err := systemClient.CloneWorkload(origSwr.Id, nil)
	be.NilErr(t, err)

	stopResp, err := systemClient.StopWorkload(origSwr.Id)
	be.NilErr(t, err)
	be.True(t, stopResp.Stopped)

	// Only the clone should remain, and it should be in the user namespace.
	_test.WaitFor(t, 10*time.Second, func() bool {
		return countWorkloads(t, userClient) == 1
	}, "waiting for original to stop after clone")
	be.Equal(t, 1, countWorkloads(t, userClient))

	wl, err := userClient.ListWorkloads(nil)
	be.NilErr(t, err)
	foundClone := false
	for _, agentResp := range wl {
		for _, summary := range *agentResp {
			if summary.Id == cloneResp.Id {
				foundClone = true
				be.Nonzero(t, summary.Namespace)
				if summary.Namespace != nil {
					be.Equal(t, "user", *summary.Namespace)
				}
			}
		}
	}
	be.True(t, foundClone)

	for _, node := range nexNodes {
		be.NilErr(t, node.Shutdown())
	}
}
