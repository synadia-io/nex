package credentials

import (
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/synadia-labs/nex/models"
)

const (
	USER_SEED string = "SUAMFX5DODJVXB2X26ZSBL4XUMYQOAWKU57N6T3RV5I5YM3SDVRRVSMYEU"
	USER_NKEY string = "UDQ2JOTBRWSTXYTHNBZQSLOLKTEOB2Z3X6U7QRBN6IZE5TS67P3F2NVS"
)

func TestNkeyMinter_MintRegister(t *testing.T) {
	m := NkeyMinter{NatsServers: []string{"nats://localhost:4222"}, NkeyCred: USER_NKEY}
	connData, err := m.MintRegister("agentId", "nodeId")
	be.NilErr(t, err)
	be.Equal(t, "nats://localhost:4222", connData.NatsServers[0])
	be.Equal(t, USER_NKEY, connData.NatsUserNkey)
	be.Zero(t, connData.NatsUserJwt)
	be.Zero(t, connData.NatsUserSeed)
}

func TestNkeyMinter_Mint(t *testing.T) {
	m := NkeyMinter{NatsServers: []string{"nats://localhost:4222"}, NkeyCred: USER_NKEY}
	connData, err := m.Mint(models.AgentCred, "namespace", "id")
	be.NilErr(t, err)
	be.Equal(t, "nats://localhost:4222", connData.NatsServers[0])
	be.Equal(t, USER_NKEY, connData.NatsUserNkey)
	be.Zero(t, connData.NatsUserJwt)
	be.Zero(t, connData.NatsUserSeed)
}
