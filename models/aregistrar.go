package models

type AgentRegistrar interface {
	RegisterRemoteInit(*RegisterRemoteAgentRequest) error
	RegisterAgent(*RegisterAgentRequest) error
}
