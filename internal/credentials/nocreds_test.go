package credentials

import (
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/synadia-labs/nex/models"
)

func TestFullAccessMinter_MintRegister(t *testing.T) {
	m := FullAccessMinter{NatsServer: "nats://localhost:4222"}
	connData, err := m.MintRegister("agentId", "nodeId")
	be.NilErr(t, err)
	be.Equal(t, "nats://localhost:4222", connData.NatsUrl)
	be.Zero(t, connData.NatsUserJwt)
	be.Zero(t, connData.NatsUserSeed)
}

func TestFullAccessMinter_Mint(t *testing.T) {
	m := FullAccessMinter{NatsServer: "nats://localhost:4222"}
	connData, err := m.Mint(models.AgentCred, "namespace", "id")
	be.NilErr(t, err)
	be.Equal(t, "nats://localhost:4222", connData.NatsUrl)
	be.Zero(t, connData.NatsUserJwt)
	be.Zero(t, connData.NatsUserSeed)
}
