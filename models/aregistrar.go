package models

type AgentRegistrar interface {
	// RegisterRemoteInit registers a remote agent with the given metadata.
	// This is called when a request is sent to the RREGISTER subject
	// headers -> header map from the NATS request
	// req -> request containing the agent register request data
	RegisterRemoteInit(headers map[string][]string, req *RegisterRemoteAgentRequest) error

	// RegisterAgent registers a remote agent with the given metadata.
	// This is called when a request is sent to the REGISTER subject
	// headers -> header map from the NATS request
	// req -> request containing the agent register request data
	RegisterAgent(headers map[string][]string, req *RegisterAgentRequest) error
}
