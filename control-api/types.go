package controlapi

import (
	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/sirupsen/logrus"
)

const (
	APIPrefix = "$NEX"
)

const (
	InfoResponseType = "io.nats.nex.v1.info_response"
	PingResponseType = "io.nats.nex.v1.ping_response"
	RunResponseType  = "io.nats.nex.v1.run_response"
	StopResponseType = "io.nats.nex.v1.stop_response"
	TagOS            = "nex.os"
	TagArch          = "nex.arch"
	TagCPUs          = "nex.cpucount"
)

type RunResponse struct {
	Started   bool   `json:"started"`
	MachineId string `json:"machine_id"`
	Issuer    string `json:"issuer"`
	Name      string `json:"name"`
}

type PingResponse struct {
	NodeId          string            `json:"node_id"`
	Version         string            `json:"version"`
	Uptime          string            `json:"uptime"`
	RunningMachines int               `json:"running_machines"`
	Tags            map[string]string `json:"tags,omitempty"`
}

type MemoryStat struct {
	MemTotal     int `json:"total"`
	MemFree      int `json:"free"`
	MemAvailable int `json:"available"`
}

type InfoResponse struct {
	Version    string            `json:"version"`
	Uptime     string            `json:"uptime"`
	PublicXKey string            `json:"public_xkey"`
	Tags       map[string]string `json:"tags,omitempty"`
	Memory     *MemoryStat       `json:"memory,omitempty"`
	Machines   []MachineSummary  `json:"machines"`
}

type MachineSummary struct {
	Id       string          `json:"id"`
	Healthy  bool            `json:"healthy"`
	Uptime   string          `json:"uptime"`
	Workload WorkloadSummary `json:"workload,omitempty"`
}

type WorkloadSummary struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Runtime      string `json:"runtime"`
	WorkloadType string `json:"type"`
	Hash         string `json:"hash"`
}

type Envelope struct {
	PayloadType string      `json:"type"`
	Data        interface{} `json:"data,omitempty"`
	Error       interface{} `json:"error,omitempty"`
}

// Wrapper for what goes across the wire
type EmittedLog struct {
	Namespace string `json:"namespace"`
	NodeId    string `json:"node_id"`
	Workload  string `json:"workload_id"`
	Timestamp string `json:"timestamp"`
	rawLog
}

type rawLog struct {
	Text      string       `json:"text"`
	Level     logrus.Level `json:"level"`
	MachineId string       `json:"machine_id"`
}

// Note this a wrapper to add context to a cloud event
type EmittedEvent struct {
	cloudevents.Event
	Namespace string `json:"namespace"`
	EventType string `json:"event_type"`
}

func NewEnvelope(dataType string, data interface{}, err *string) Envelope {
	var e interface{}
	if err != nil {
		e = *err
	}
	return Envelope{
		PayloadType: dataType,
		Data:        data,
		Error:       e,
	}
}
