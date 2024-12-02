package agentcommon

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/api/agent/go"
	agentapigen "github.com/synadia-io/nex/api/agent/go/gen"
)

// An agent client is used by a Nex node host to communicate with
// an agent.
type AgentClient struct {
	internalNatsConn *nats.Conn
	agentName        string
}

func NewAgentClient(nc *nats.Conn, name string) (*AgentClient, error) {
	return &AgentClient{internalNatsConn: nc, agentName: name}, nil
}

func (ac *AgentClient) StartWorkload(req *agentapigen.StartWorkloadRequestJson) error {
	subject := agentapi.StartWorkloadSubject(ac.agentName)
	payload, err := json.Marshal(&req)
	if err != nil {
		return err
	}

	res, err := ac.internalNatsConn.Request(subject, payload, 2*time.Second)
	if err != nil {
		return err
	}

	var result agentapigen.StartWorkloadResponseJson
	err = json.Unmarshal(res.Data, &result)
	if err != nil {
		return err
	}

	if !result.Success && result.Error != nil {
		return errors.New(*result.Error)
	} else if !result.Success {
		return errors.New("unknown failure starting workload")
	}
	return nil
}

func (ac *AgentClient) StopWorkload(workloadId string) error {
	subject := agentapi.StopWorkloadSubject(workloadId)
	immediate := true  // TODO: accept options
	reason := "normal" // TODO: accept options

	req := &agentapigen.StopWorkloadRequestJson{
		Immediate:  &immediate,
		Reason:     &reason,
		WorkloadId: workloadId,
	}
	payload, err := json.Marshal(&req)
	if err != nil {
		return err
	}
	res, err := ac.internalNatsConn.Request(subject, payload, 2*time.Second)
	if err != nil {
		return err
	}
	var response agentapigen.StopWorkloadResponseJson
	err = json.Unmarshal(res.Data, &response)
	if err != nil {
		return err
	}

	if !response.Stopped {
		return errors.New("Agent indicated workload did not stop")
	}

	return nil
}

func (ac *AgentClient) TriggerWorkload(workloadId string, payload []byte) ([]byte, error) {
	// TODO: implement

	return nil, nil
}
