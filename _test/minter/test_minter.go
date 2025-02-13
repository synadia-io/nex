package testminter

import "github.com/synadia-labs/nex/models"

type TestMinter struct {
	NatsServer string
}

func (m *TestMinter) MintRegister(agentId, nodeId string) (*models.NatsConnectionData, error) {
	ret := new(models.NatsConnectionData)
	ret.NatsUrl = m.NatsServer
	return ret, nil
}

func (m *TestMinter) Mint(typ models.CredType, namespace, id string) (*models.NatsConnectionData, error) {
	ret := new(models.NatsConnectionData)
	ret.NatsUrl = m.NatsServer
	return ret, nil
}
