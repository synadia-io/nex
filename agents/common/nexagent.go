package agentcommon

import "github.com/nats-io/nats.go"

// A convenient bundling for common functions that belong to a Nex Agent
type NexAgent struct {
	upHandler        CommandHandler
	preflightHandler CommandHandler
	embeddedNats     *nats.Conn
}

func NewNexAgent(up CommandHandler, preflight CommandHandler) (*NexAgent, error) {
	// TODO: generate whatever context we need

	embedded, err := CreateEmbeddedNatsConnection()
	if err != nil {
		return nil, err
	}
	return &NexAgent{upHandler: up, preflightHandler: preflight, embeddedNats: embedded}, nil
}

func (agent *NexAgent) Run() error {
	// TODO: based on argv, decide which handler to call
	// get error from handler and return here
	return nil
}

func (agent *NexAgent) NewHostServicesConnection(workloadId string, host string, port int, jwt string, seed string) (*nats.Conn, error) {
	return createHostServicesConnection(workloadId, host, port, jwt, seed)
}

// TODO: this should actually get passed the node configuration options to make
// the preflight check accurate
type CommandHandler func() error
