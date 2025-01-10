package models

type CredType int

const (
	AgentCred CredType = iota
	WorkloadCred
)

type CredVendor interface {
	MintRegister(agentId, nodeId string) (*NatsConnectionData, error)
	Mint(typ CredType, namespace, id string) (*NatsConnectionData, error)
}
