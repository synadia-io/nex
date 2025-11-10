package credentials

import (
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
)

type Credential struct {
	Jwt      string
	NkeySeed []byte
}

func (c *Credential) String() string {
	return `-----BEGIN NATS USER JWT-----
` + c.Jwt + `
------END NATS USER JWT------

************************* IMPORTANT *************************
NKEY Seed printed below can be used to sign and prove identity.
NKEYs are sensitive and should be treated as secrets.

-----BEGIN USER NKEY SEED-----
` + string(c.NkeySeed) + `
------END USER NKEY SEED------

*************************************************************
`
}

type SigningKeyMinter struct {
	NodeId         string
	NatsServers    []string
	Nexus          string
	RootAccountKey string
	SigningSeed    string
}

func (m *SigningKeyMinter) MintRegister(agentId, nodeId string) (*models.NatsConnectionData, error) {
	pkp, err := nkeys.FromSeed([]byte(m.SigningSeed))
	if err != nil {
		return nil, err
	}

	ret := new(models.NatsConnectionData)
	ret.NatsServers = m.NatsServers

	kp, err := nkeys.CreateUser()
	if err != nil {
		return nil, err
	}
	seed, err := kp.Seed()
	if err != nil {
		return nil, err
	}

	ret.NatsUserSeed = string(seed)

	pubKp, err := kp.PublicKey()
	if err != nil {
		return nil, err
	}

	claims := jwt.NewUserClaims(pubKp)
	claims.Subject = pubKp
	claims.Expires = time.Now().Add(time.Hour * 24 * 365).Unix()
	claims.Name = agentId
	claims.IssuerAccount = m.RootAccountKey
	claims.Permissions = AgentRegistrationClaims(agentId, nodeId)

	vr := jwt.CreateValidationResults()
	claims.Validate(vr)

	ret.NatsUserJwt, err = claims.Encode(pkp)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func (m *SigningKeyMinter) Mint(typ models.CredType, namespace, id string) (*models.NatsConnectionData, error) {
	pkp, err := nkeys.FromSeed([]byte(m.SigningSeed))
	if err != nil {
		return nil, err
	}

	ret := new(models.NatsConnectionData)
	ret.NatsServers = m.NatsServers

	kp, err := nkeys.CreateUser()
	if err != nil {
		return nil, err
	}
	seed, err := kp.Seed()
	if err != nil {
		return nil, err
	}

	ret.NatsUserSeed = string(seed)

	pubKp, err := kp.PublicKey()
	if err != nil {
		return nil, err
	}

	claims := jwt.NewUserClaims(pubKp)
	claims.Subject = pubKp
	claims.Expires = time.Now().Add(time.Hour * 24 * 365).Unix()
	claims.Name = id
	claims.IssuerAccount = m.RootAccountKey

	switch typ {
	case models.AgentCred:
		claims.Permissions = AgentClaims(id, m.NodeId, m.Nexus)
	case models.WorkloadCred:
		claims.Permissions = WorkloadClaims(namespace, id)
	}

	vr := jwt.CreateValidationResults()
	claims.Validate(vr)

	ret.NatsUserJwt, err = claims.Encode(pkp)
	if err != nil {
		return nil, err
	}

	return ret, nil
}
