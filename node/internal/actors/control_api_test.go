package actors

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/tochemey/goakt/v3/testkit"

	api "github.com/synadia-io/nex/api/go"
	"github.com/synadia-io/nex/models"
)

func TestControlApiAgent(t *testing.T) {
	workingDir := t.TempDir()

	s := startNatsServer(t, workingDir)
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	pubKey, err := kp.PublicKey()
	be.NilErr(t, err)

	ctlApi := CreateControlAPI(nc, slog.New(slog.NewTextHandler(os.Stdout, nil)), pubKey, NewMockNode())
	as := CreateAgentSupervisor(models.NodeOptions{})

	ctx := context.Background()
	tk := testkit.New(ctx, t)
	tk.Spawn(ctx, AgentSupervisorActorName, as)
	tk.Spawn(ctx, ControlAPIActorName, ctlApi)
	time.Sleep(250 * time.Millisecond)
	defer tk.Shutdown(ctx)

	t.Run("TestAuctionHandler", func(t *testing.T) {
		req := api.AuctionRequestJson{
			AgentType: []api.NexWorkload{"test"},
			AuctionId: "foo",
			Tags: api.AuctionRequestJsonTags{
				Tags: make(map[string]string),
			},
		}

		reqB, err := json.Marshal(req)
		be.NilErr(t, err)

		respInbox := nats.NewInbox()
		respStream := new(bytes.Buffer)
		resp, err := nc.Subscribe(respInbox, func(m *nats.Msg) {
			respStream.Write(m.Data)
		})
		be.NilErr(t, err)

		msg := nats.NewMsg(models.AuctionRequestSubject("system"))
		msg.Reply = respInbox
		msg.Data = reqB
		msg.Sub = resp
		msg.Subject = models.AuctionRequestSubject("system")

		ctlApi.handleAuction(msg)
		time.Sleep(time.Second)

		be.Nonzero(t, respStream.Bytes())
		auctionReply := new(models.Envelope[api.AuctionResponseJson])
		be.NilErr(t, json.Unmarshal(respStream.Bytes(), auctionReply))

		be.Equal(t, auctionReply.Data.BidderId, "abc123")
		be.NilErr(t, resp.Unsubscribe())
	})

	t.Run("TestPingHandler", func(t *testing.T) {
		respInbox := nats.NewInbox()
		respStream := new(bytes.Buffer)
		resp, err := nc.Subscribe(respInbox, func(m *nats.Msg) {
			respStream.Write(m.Data)
		})
		be.NilErr(t, err)

		msg := nats.NewMsg(models.PingSubject())
		msg.Reply = respInbox
		msg.Sub = resp
		msg.Subject = models.PingSubject()

		ctlApi.handlePing(msg)
		time.Sleep(time.Second)

		be.Nonzero(t, respStream.Bytes())
		nodePingReply := new(models.Envelope[api.NodePingResponseJson])
		be.NilErr(t, json.Unmarshal(respStream.Bytes(), nodePingReply))
		be.Equal(t, "NNODE123", nodePingReply.Data.NodeId)
		be.Equal(t, "TESTNEXUS", nodePingReply.Data.Tags.Tags[models.TagNexus])
		be.Equal(t, "0.0.0", nodePingReply.Data.Version)
		be.Equal(t, "mock-node", nodePingReply.Data.Tags.Tags[models.TagNodeName])
	})
}
