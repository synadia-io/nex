package services

import "github.com/nats-io/nats.go"

type HostService interface {
	HandleRPC(msg *nats.Msg)
}
