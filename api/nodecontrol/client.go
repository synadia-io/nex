package nodecontrol

import (
	"encoding/json"
	"log/slog"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nuid"
	nodegen "github.com/synadia-io/nex/api/nodecontrol/gen"
	"github.com/synadia-io/nex/models"
)

var (
	DefaultRequestTimeout = 10 * time.Second
)

type ControlAPIClient struct {
	nc     *nats.Conn
	logger *slog.Logger
}

func NewControlApiClient(nc *nats.Conn, logger *slog.Logger) (*ControlAPIClient, error) {
	return &ControlAPIClient{
		nc:     nc,
		logger: logger,
	}, nil
}

func (c *ControlAPIClient) Auction(namespace string, tags map[string]string) ([]*nodegen.AuctionResponseJson, error) {
	resp := []*nodegen.AuctionResponseJson{}
	auctionRespInbox := nats.NewInbox()

	_, err := c.nc.Subscribe(auctionRespInbox, func(m *nats.Msg) {
		envelope := new(models.Envelope[nodegen.AuctionResponseJson])
		err := json.Unmarshal(m.Data, envelope)
		if err != nil {
			c.logger.Error("failed to unmarshal auction response", slog.Any("err", err), slog.String("data", string(m.Data)))
			return
		}
		resp = append(resp, &envelope.Data)
	})
	if err != nil {
		return nil, err
	}

	req := nodegen.AuctionRequestJson{
		AuctionId: nuid.New().Next(),
		Tags: nodegen.AuctionRequestJsonTags{
			Tags: tags,
		},
	}
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	err = c.nc.PublishRequest(models.AuctionRequestSubject(namespace), auctionRespInbox, req_b)
	if err != nil {
		return nil, err
	}

	time.Sleep(5 * time.Second)
	return resp, nil
}

func (c *ControlAPIClient) Ping() ([]*nodegen.NodePingResponseJson, error) {
	resp := []*nodegen.NodePingResponseJson{}
	pingRespInbox := nats.NewInbox()

	_, err := c.nc.Subscribe(pingRespInbox, func(m *nats.Msg) {
		envelope := new(models.Envelope[nodegen.NodePingResponseJson])
		err := json.Unmarshal(m.Data, envelope)
		if err != nil {
			c.logger.Debug("failed to unmarshal ping response", slog.Any("err", err), slog.String("data", string(m.Data)))
			return
		}
		resp = append(resp, &envelope.Data)
	})
	if err != nil {
		return nil, err
	}

	err = c.nc.PublishRequest(models.PingSubject(), pingRespInbox, nil)
	if err != nil {
		return nil, err
	}

	time.Sleep(5 * time.Second)
	return resp, nil
}

func (c *ControlAPIClient) DirectPing(nodeId string) (*nodegen.NodePingResponseJson, error) {
	msg, err := c.nc.Request(models.DirectPingSubject(nodeId), nil, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}
	resp := new(nodegen.NodePingResponseJson)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) FindWorkload(inType, namespace, workloadId string) (*nodegen.WorkloadPingResponseJson, error) {
	msg, err := c.nc.Request(models.WorkloadPingRequestSubject(inType, namespace, workloadId), nil, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	envelope := new(models.Envelope[nodegen.WorkloadPingResponseJson])
	err = json.Unmarshal(msg.Data, envelope)
	if err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

func (c *ControlAPIClient) AuctionDeployWorkload(namespace, bidderId string, req nodegen.StartWorkloadRequestJson) (*nodegen.StartWorkloadResponseJson, error) {
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	msg, err := c.nc.Request(models.AuctionDeployRequestSubject(namespace, bidderId), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	envelope := new(models.Envelope[nodegen.StartWorkloadResponseJson])
	err = json.Unmarshal(msg.Data, envelope)
	if err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

func (c *ControlAPIClient) DeployWorkload(namespace, nodeId string, req nodegen.StartWorkloadRequestJson) (*nodegen.StartWorkloadResponseJson, error) {
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(models.DirectDeploySubject(nodeId), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	envelope := new(models.Envelope[nodegen.StartWorkloadResponseJson])
	err = json.Unmarshal(msg.Data, envelope)
	if err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

func (c *ControlAPIClient) UndeployWorkload(namespace, nodeId, workloadId string, req nodegen.StopWorkloadRequestJson) (*nodegen.StopWorkloadResponseJson, error) {
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(models.UndeployRequestSubject(namespace, nodeId), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	envelope := new(models.Envelope[nodegen.StopWorkloadResponseJson])
	err = json.Unmarshal(msg.Data, envelope)
	if err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

func (c *ControlAPIClient) GetInfo(nodeId, namespace string) (*nodegen.NodeInfoResponseJson, error) {
	msg, err := c.nc.Request(models.InfoRequestSubject(namespace, nodeId), nil, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	envelope := new(models.Envelope[nodegen.NodeInfoResponseJson])
	err = json.Unmarshal(msg.Data, envelope)
	if err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

func (c *ControlAPIClient) SetLameDuck(nodeId string, delay time.Duration) (*nodegen.LameduckResponseJson, error) {
	req := nodegen.LameduckRequestJson{
		Delay: delay.String(),
	}

	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(models.LameduckSubject(nodeId), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	envelope := new(models.Envelope[nodegen.LameduckResponseJson])
	err = json.Unmarshal(msg.Data, envelope)
	if err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

func (c *ControlAPIClient) MonitorLogs(workloadId, level string) (chan []byte, error) {
	subject := models.LOGS_SUBJECT
	f_subject, err := subject.Filter(workloadId, level)
	if err != nil {
		return nil, err
	}

	ret := make(chan []byte)
	_, err = c.nc.Subscribe(f_subject, func(msg *nats.Msg) {
		ret <- msg.Data
	})
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func (c *ControlAPIClient) MonitorEvents(workloadId, eventType string) (chan *cloudevents.Event, error) {
	subject := models.EVENTS_SUBJECT
	f_subject, err := subject.Filter(workloadId, eventType)
	if err != nil {
		return nil, err
	}

	ret := make(chan *cloudevents.Event)
	_, err = c.nc.Subscribe(f_subject, func(msg *nats.Msg) {
		e := new(cloudevents.Event)
		err := json.Unmarshal(msg.Data, e)
		if err != nil {
			c.logger.Error("failed to unmarshal cloud event", slog.Any("err", err), slog.String("data", string(msg.Data)))
			return
		}
		ret <- e
	})
	if err != nil {
		return nil, err
	}

	return ret, nil
}
