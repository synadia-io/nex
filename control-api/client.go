package controlapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/nats-io/nats.go"
)

// API subjects:
// $NEX.PING
// $NEX.PING.{node}
// $NEX.INFO.{namespace}.{node}
// $NEX.RUN.{namespace}.{node}
// $NEX.STOP.{namespace}.{node}
// $NEX.LAMEDUCK.{node}

// A control API client communicates with a "Nexus" of nodes by virtue of the $NEX.> subject space. This
// client should be used to communicate with Nex nodes whenever possible, and its patterns should be copied
// for clients in other languages. Requests made to the $NEX.> subject space are, when appropriate, secured
// via signed JWTs
type Client struct {
	nc        *nats.Conn
	timeout   time.Duration
	namespace string
	log       *slog.Logger
}

// Creates a new client to communicate with a group of NEX nodes, using the
// namespace of 'default' for applicable requests
func NewApiClient(nc *nats.Conn, timeout time.Duration, log *slog.Logger) *Client {
	return NewApiClientWithNamespace(nc, timeout, "default", log)
}

// Creates a new client to communicate with a group of Nex nodes with workloads scoped to the
// given namespace. Note that this namespace is used for requests where it is mandatory
func NewApiClientWithNamespace(nc *nats.Conn, timeout time.Duration, namespace string, log *slog.Logger) *Client {
	return &Client{nc: nc, timeout: timeout, namespace: namespace, log: log}
}

// Attempts to stop a running workload. This can fail for a wide variety of reasons, the most common
// is likely to be security validation that prevents one issuer from submitting a stop request for
// another issuer's workload
func (api *Client) StopWorkload(stopRequest *StopRequest) (*StopResponse, error) {
	subject := fmt.Sprintf("%s.STOP.%s.%s", APIPrefix, api.namespace, stopRequest.TargetNode)
	bytes, err := api.performRequest(subject, stopRequest)
	if err != nil {
		return nil, err
	}

	var response StopResponse
	err = json.Unmarshal(bytes, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil

}

// Attempts to start a workload. The workload URI, at the moment, must always point to a NATS object store
// bucket in the form of `nats://{bucket}/{key}`. Note that JetStream domains can be supplied on the workload
// request and aren't part of the bucket+key URL.
func (api *Client) StartWorkload(request *DeployRequest) (*RunResponse, error) {
	subject := fmt.Sprintf("%s.DEPLOY.%s.%s", APIPrefix, api.namespace, *request.TargetNode)
	bytes, err := api.performRequest(subject, request)
	if err != nil {
		return nil, err
	}

	var response RunResponse
	err = json.Unmarshal(bytes, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// Requests information for a given node within the client's namespace
func (api *Client) NodeInfo(nodeId string) (*InfoResponse, error) {
	subject := fmt.Sprintf("%s.INFO.%s.%s", APIPrefix, api.namespace, nodeId)
	bytes, err := api.performRequest(subject, nil)
	if err != nil {
		return nil, err
	}

	var response InfoResponse
	err = json.Unmarshal(bytes, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (api *Client) EnterLameDuck(nodeId string) (*LameDuckResponse, error) {
	subject := fmt.Sprintf("%s.LAMEDUCK.%s", APIPrefix, nodeId)
	bytes, err := api.performRequest(subject, nil)
	if err != nil {
		return nil, err
	}

	var response LameDuckResponse
	err = json.Unmarshal(bytes, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// This is a filtered node ping that returns only matching workloads.
// A workloadId of "" will not filter by workload, and only
// filter by the client's namespace. If a workload ID/name is supplied, the filter
// will be for both namespace and workload. If you don't want these filters
// then use ListAllNodes
func (api *Client) ListWorkloads(workloadId string) ([]WorkloadPingResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), api.timeout)
	defer cancel()

	workloadId = strings.TrimSpace(workloadId)

	responses := make([]WorkloadPingResponse, 0)

	sub, err := api.nc.Subscribe(api.nc.NewRespInbox(), func(m *nats.Msg) {
		env, err := extractEnvelope(m.Data)
		if err != nil {
			return
		}
		var resp WorkloadPingResponse
		bytes, err := json.Marshal(env.Data)
		if err != nil {
			return
		}
		err = json.Unmarshal(bytes, &resp)
		if err != nil {
			return
		}
		responses = append(responses, resp)
	})
	if err != nil {
		return nil, err
	}

	var subject string
	if len(workloadId) == 0 {
		subject = fmt.Sprintf("%s.WPING.%s", APIPrefix, api.namespace)
	} else {
		subject = fmt.Sprintf("%s.WPING.%s.%s", APIPrefix, api.namespace, workloadId)
	}
	msg := nats.NewMsg(subject)
	msg.Reply = sub.Subject
	err = api.nc.PublishMsg(msg)
	if err != nil {
		return nil, err
	}

	<-ctx.Done()
	return responses, nil
}

// Attempts to list all nodes. Note that any node within the Nexus will respond to this ping, regardless
// of the namespaces of their running workloads
func (api *Client) ListAllNodes() ([]PingResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), api.timeout)
	defer cancel()

	responses := make([]PingResponse, 0)

	sub, err := api.nc.Subscribe(api.nc.NewRespInbox(), func(m *nats.Msg) {
		env, err := extractEnvelope(m.Data)
		if err != nil {
			api.log.Error("failed to extract envelope", slog.Any("err", err), slog.Any("nats_msg.Data", m.Data))
			return
		}

		var resp PingResponse
		bytes, err := json.Marshal(env.Data)
		if err != nil {
			api.log.Error("failed to marshal envelope data", slog.Any("err", err))
			return
		}

		err = json.Unmarshal(bytes, &resp)
		if err != nil {
			api.log.Error("failed to unmarshal PingResponse", slog.Any("err", err))
			return
		}
		responses = append(responses, resp)
	})
	if err != nil {
		api.log.Error("failed to subscribe", slog.Any("err", err))
		return nil, err
	}
	msg := nats.NewMsg(fmt.Sprintf("%s.PING", APIPrefix))
	msg.Reply = sub.Subject
	err = api.nc.PublishMsg(msg)
	if err != nil {
		return nil, err
	}

	<-ctx.Done()
	return responses, nil
}

// A convenience function that subscribes to all available logs and returns
// an unbuffered, blocking channel
func (api *Client) MonitorAllLogs() (chan EmittedLog, error) {
	return api.MonitorLogs("*", "*", "*", "*", 0)
}

// Creates a NATS subscription to the appropriate log subject. If you do not want to limit
// the monitor by any of the filters, supply a '*', not an empty string. Buffer length refers
// to the size of the channel buffer, where 0 is unbuffered (aka blocking)
func (api *Client) MonitorLogs(
	namespaceFilter string,
	nodeFilter string,
	workloadFilter string,
	vmFilter string,
	bufferLength int) (chan EmittedLog, error) {

	subject := fmt.Sprintf("%s.logs.%s.%s.%s.%s", APIPrefix,
		namespaceFilter,
		nodeFilter,
		workloadFilter,
		vmFilter)

	logChannel := make(chan EmittedLog, bufferLength)
	_, err := api.nc.Subscribe(subject, handleLogEntry(api, logChannel))
	if err != nil {
		return nil, err
	}

	return logChannel, nil
}

// A convenience function that monitors all available events without filter, and
// uses an unbuffered (blocking) channel for the results
func (api *Client) MonitorAllEvents() (chan EmittedEvent, error) {
	return api.MonitorEvents("*", "*", 0)
}

// Creates a NATS subscription to the appropriate event subject. If you don't want to limit
// the monitor to a specific namespace or event type, then supply '*' for both values, not
// an empty string. Buffer length is the size of the channel buffer, where 0 is unbuffered (blocking)
func (api *Client) MonitorEvents(
	namespaceFilter string,
	eventTypeFilter string,
	bufferLength int) (chan EmittedEvent, error) {

	subscribeSubject := fmt.Sprintf("%s.events.%s.*", APIPrefix, namespaceFilter)

	eventChannel := make(chan EmittedEvent, bufferLength)

	_, err := api.nc.Subscribe(subscribeSubject, handleEventEntry(eventChannel))
	if err != nil {
		return nil, err
	}

	// Add a monitor for the system namespace if the supplied filter doesn't
	// already include it
	if namespaceFilter != "*" && namespaceFilter != "system" {
		systemSub := fmt.Sprintf("%s.events.system.*", APIPrefix)
		_, err = api.nc.Subscribe(systemSub, handleEventEntry(eventChannel))
		if err != nil {
			return nil, err
		}
	}

	return eventChannel, nil
}

func handleEventEntry(ch chan EmittedEvent) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		tokens := strings.Split(m.Subject, ".")
		if len(tokens) != 4 {
			return
		}

		namespace := tokens[2]
		eventType := tokens[3]

		event := cloudevents.NewEvent()
		err := json.Unmarshal(m.Data, &event)
		if err != nil {
			return
		}

		ch <- EmittedEvent{
			Event:     event,
			Namespace: namespace,
			EventType: eventType,
		}
	}
}

func handleLogEntry(api *Client, ch chan EmittedLog) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		/*
			$NEX.logs.{namespace}.{node}.{workload}.{vm}
		*/
		tokens := strings.Split(m.Subject, ".")
		if len(tokens) != 6 {
			api.log.Debug("token length not 6", "length", len(tokens), "subject", m.Subject)
			return
		}

		var logEntry RawLog
		err := json.Unmarshal(m.Data, &logEntry)
		if err != nil {
			api.log.Error("Log entry deserialization failure", err)
			return
		}

		ch <- EmittedLog{
			Namespace: tokens[2],
			NodeId:    tokens[3],
			Workload:  tokens[4],
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RawLog:    logEntry,
		}
	}

}

// Helper that submits data, gets a standard envelope back, and returns the inner data
// payload as JSON
func (api *Client) performRequest(subject string, raw interface{}) ([]byte, error) {
	var bytes []byte
	var err error
	if raw == nil {
		bytes = []byte{}
	} else {
		bytes, err = json.Marshal(raw)
		if err != nil {
			return nil, err
		}
	}

	resp, err := api.nc.Request(subject, bytes, api.timeout)
	if err != nil {
		return nil, err
	}
	env, err := extractEnvelope(resp.Data)
	if err != nil {
		return nil, err
	}
	if env.Error != nil {
		return nil, fmt.Errorf("%v", env.Error)
	}
	return json.Marshal(env.Data)
}

func extractEnvelope(data []byte) (*Envelope, error) {
	var env Envelope
	err := json.Unmarshal(data, &env)
	if err != nil {
		return nil, err
	}
	return &env, nil
}
