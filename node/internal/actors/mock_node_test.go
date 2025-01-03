package actors

import (
	"context"
	"encoding/json"

	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ NodeCallback = (*mockNode)(nil)

type mockNode struct {
	options *models.NodeOptions
}

func NewMockNode() *mockNode {
	return &mockNode{
		options: &models.NodeOptions{
			Tags: map[string]string{models.TagNodeName: "mock-node", models.TagNexus: "TESTNEXUS"},
		},
	}
}

func (n *mockNode) Auction(auctionId string, agentType []string, tags map[string]string) (*actorproto.AuctionResponse, error) {
	return &actorproto.AuctionResponse{
		BidderId:   "abc123",
		Version:    "0.0.0",
		TargetXkey: "XNODEKEY",
		StartedAt:  timestamppb.Now(),
		Tags:       n.options.Tags,
		Status:     make(map[string]int32),
	}, nil
}

func (n mockNode) Ping() (*actorproto.PingNodeResponse, error) {
	return &actorproto.PingNodeResponse{
		NodeId:        "NNODE123",
		Version:       "0.0.0",
		TargetXkey:    "XNODEXKEY",
		StartedAt:     timestamppb.Now(),
		Tags:          n.options.Tags,
		RunningAgents: make(map[string]int32),
	}, nil
}

func (n mockNode) GetInfo(string) (*actorproto.NodeInfo, error) {
	return nil, nil
}

func (n *mockNode) SetLameDuck(context.Context) {
	n.options.Tags[models.TagLameDuck] = "true"
}

func (n mockNode) IsTargetNode(string) (bool, nkeys.KeyPair, error) {
	return false, nil, nil
}

func (n mockNode) EncryptPayload([]byte, string) ([]byte, string, error) {
	return nil, "", nil
}

func (n mockNode) DecryptPayload([]byte) ([]byte, error) {
	return nil, nil
}

func (n mockNode) EmitEvent(string, json.RawMessage) error {
	return nil
}

func (n mockNode) StoreRunRequest(string, string, *actorproto.StartWorkload) error {
	return nil
}

func (n mockNode) GetRunRequest(string, string) (*actorproto.StartWorkload, error) {
	return nil, nil
}

func (n mockNode) DeleteRunRequest(string, string) error {
	return nil
}

func (n mockNode) StartWorkloadMessage() string { return "" }
func (n mockNode) StopWorkloadMessage() string  { return "" }
