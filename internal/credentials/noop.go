package credentials

import "github.com/synadia-labs/nex/models"

type FullAccessMinter struct {
	NatsServer string
}

func (m *FullAccessMinter) MintRegister(agentId, nodeId string) (*models.NatsConnectionData, error) {
	ret := new(models.NatsConnectionData)
	ret.NatsUrl = m.NatsServer
	return ret, nil
}

func (m *FullAccessMinter) Mint(typ models.CredType, namespace, id string) (*models.NatsConnectionData, error) {
	ret := new(models.NatsConnectionData)
	ret.NatsUrl = m.NatsServer
	return ret, nil
}
