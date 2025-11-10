package testminter

import "github.com/synadia-io/nex/models"

type TestMinter struct {
	NatsServers []string
}

func (m *TestMinter) MintRegister(agentId, nodeId string) (*models.NatsConnectionData, error) {
	ret := new(models.NatsConnectionData)
	ret.NatsServers = m.NatsServers
	return ret, nil
}

func (m *TestMinter) Mint(typ models.CredType, namespace, id string) (*models.NatsConnectionData, error) {
	ret := new(models.NatsConnectionData)
	ret.NatsServers = m.NatsServers
	return ret, nil
}
