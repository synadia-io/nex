package credentials

import "github.com/synadia-labs/nex/models"

type NkeyMinter struct {
	NatsServers []string
	NkeyCred    string
	NkeySeed    string
}

func (m *NkeyMinter) MintRegister(agentId, nodeId string) (*models.NatsConnectionData, error) {
	ret := new(models.NatsConnectionData)
	ret.NatsServers = m.NatsServers
	ret.NatsUserNkey = m.NkeyCred
	ret.NatsUserSeed = m.NkeySeed
	return ret, nil
}

func (m *NkeyMinter) Mint(typ models.CredType, namespace, id string) (*models.NatsConnectionData, error) {
	ret := new(models.NatsConnectionData)
	ret.NatsServers = m.NatsServers
	ret.NatsUserNkey = m.NkeyCred
	ret.NatsUserSeed = m.NkeySeed
	return ret, nil
}
