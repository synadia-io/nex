package credentials

import (
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/synadia-labs/nex/models"
)

const (
	SigningKey        string = "SAAO4BQQIG6ESCYGHXTBEEF4TWX7XI545EAYZPXZMR5JCIRUZLRWUONGD4"
	SigningKeyAccount string = "ABOMDDCH76P5CAEOFEC5AFRMUL3W62Y5SPBNL6R3GBYE5X4N6UDE5QQL"
)

func TestSigningKeyMinter_MintRegister(t *testing.T) {
	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)
	kpPub, err := kp.PublicKey()
	be.NilErr(t, err)

	m := SigningKeyMinter{
		NodeId:         kpPub,
		Nexus:          "nexus",
		NatsServers:    []string{"nats://localhost:4222"},
		RootAccountKey: SigningKeyAccount,
		SigningSeed:    SigningKey,
	}
	connData, err := m.MintRegister("agentId", "nodeId")
	be.NilErr(t, err)
	be.Equal(t, "nats://localhost:4222", connData.NatsServers[0])
	be.Nonzero(t, connData.NatsUserSeed)

	claims, err := jwt.DecodeUserClaims(connData.NatsUserJwt)
	be.NilErr(t, err)

	genClaims := AgentRegistrationClaims("agentId", "nodeId")
	be.AllEqual(t, genClaims.Pub.Allow, claims.Pub.Allow)
	be.AllEqual(t, genClaims.Pub.Deny, claims.Pub.Deny)
	be.AllEqual(t, genClaims.Sub.Allow, claims.Sub.Allow)
	be.AllEqual(t, genClaims.Sub.Deny, claims.Sub.Deny)
}

func TestSigningKeyMinter_Mint(t *testing.T) {
	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)
	kpPub, err := kp.PublicKey()
	be.NilErr(t, err)

	tt := []struct {
		name  string
		t     models.CredType
		ns    string
		id    string
		perms jwt.Permissions
	}{
		{"Agent Cred", models.AgentCred, "", "agentId", AgentClaims("agentId", kpPub, "nexus")},
		{"Workload Cred", models.WorkloadCred, "user", "workloadId", WorkloadClaims("user", "workloadId")},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			m := SigningKeyMinter{
				NodeId:         kpPub,
				Nexus:          "nexus",
				NatsServers:    []string{"nats://localhost:4222"},
				RootAccountKey: SigningKeyAccount,
				SigningSeed:    SigningKey,
			}
			connData, err := m.Mint(tc.t, tc.ns, tc.id)
			be.NilErr(t, err)
			be.Equal(t, "nats://localhost:4222", connData.NatsServers[0])
			be.Nonzero(t, connData.NatsUserSeed)

			claims, err := jwt.DecodeUserClaims(connData.NatsUserJwt)
			be.NilErr(t, err)

			be.AllEqual(t, tc.perms.Pub.Allow, claims.Pub.Allow)
			be.AllEqual(t, tc.perms.Pub.Deny, claims.Pub.Deny)
			be.AllEqual(t, tc.perms.Sub.Allow, claims.Sub.Allow)
			be.AllEqual(t, tc.perms.Sub.Deny, claims.Sub.Deny)
		})
	}
}
