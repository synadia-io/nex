package controlapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// API subjects:
// $NEX.PING
// $NEX.PING.{node}
// $NEX.INFO.{namespace}.{node}
// $NEX.RUN.{namespace}.{node}
// $NEX.STOP.{namespace}.{node}

type Client struct {
	nc        *nats.Conn
	timeout   time.Duration
	namespace string
	log       *logrus.Logger
}

// Creates a new client to communicate with a group of NEX nodes, using the
// namespace of 'default' for applicable requests
func NewApiClient(nc *nats.Conn, timeout time.Duration, log *logrus.Logger) *Client {
	return NewApiClientWithNamespace(nc, timeout, "default", log)
}

// Creates a new client to communicate with a group of NEX nodes all within a given namespace. Note that
// this namespace is used for requests where it is mandatory
func NewApiClientWithNamespace(nc *nats.Conn, timeout time.Duration, namespace string, log *logrus.Logger) *Client {
	return &Client{nc: nc, timeout: timeout, namespace: namespace, log: log}
}

// Attempts to stop a running workload. This can fail for a wide variety of reasons, the most common
// is likely to be security validation that prevents one issuer from issuing a stop request for
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
// bucket in the form of `nats://{bucket}/{key}`
func (api *Client) StartWorkload(request *RunRequest) (*RunResponse, error) {
	subject := fmt.Sprintf("%s.RUN.%s.%s", APIPrefix, api.namespace, request.TargetNode)
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

// Attempts to list all nodes. Note that this operation returns all visible nodes regardless of
// namespace
func (api *Client) ListNodes() ([]PingResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), api.timeout)
	defer cancel()

	responses := make([]PingResponse, 0)

	sub, err := api.nc.Subscribe(api.nc.NewRespInbox(), func(m *nats.Msg) {
		env, err := extractEnvelope(m.Data)
		if err != nil {
			return
		}
		var resp PingResponse
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
		return nil, nil
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

// A convenience function that subscribes to all available logs and uses
// an unbuffered, blocking channel
func (api *Client) MonitorAllLogs() (chan EmittedLog, error) {
	return api.MonitorLogs("*", "*", "*", "*", 0)
}

// Creates a NATS subscription to the appropriate log subject. If you do not want to limit
// the monitor by any of the filters, supply a '*', not an empty string. Bufferlength refers
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

	_, err := api.nc.Subscribe(subscribeSubject, handleEventEntry(api, eventChannel))
	if err != nil {
		return nil, err
	}

	// Add a monitor for the system namespace if the supplied filter doesn't
	// already include it
	if namespaceFilter != "*" && namespaceFilter != "system" {
		systemSub := fmt.Sprintf("%s.events.system.*", APIPrefix)
		_, err = api.nc.Subscribe(systemSub, handleEventEntry(api, eventChannel))
		if err != nil {
			return nil, err
		}
	}

	return eventChannel, nil
}

func handleEventEntry(api *Client, ch chan EmittedEvent) func(m *nats.Msg) {
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
			return
		}
		var logEntry rawLog
		err := json.Unmarshal(m.Data, &logEntry)
		if err != nil {
			api.log.WithError(err).Error("Log entry deserialization failure")
			return
		}
		if logEntry.Level == 0 {
			logEntry.Level = logrus.DebugLevel
		}

		ch <- EmittedLog{
			Namespace: tokens[2],
			NodeId:    tokens[3],
			Workload:  tokens[4],
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			rawLog:    logEntry,
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
