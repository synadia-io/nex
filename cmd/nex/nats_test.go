package main

import (
	"testing"

	"github.com/carlmjohnson/be"
)

func TestConfigureNatsConnection(t *testing.T) {
	s := startNatsServer(t)
	defer s.Shutdown()

	cfg := &Globals{
		GlobalNats: GlobalNats{
			NatsContext:         "",
			NatsServers:         []string{s.ClientURL()},
			NatsUserNkey:        "",
			NatsUserSeed:        "",
			NatsUserJWT:         "",
			NatsUser:            "",
			NatsUserPassword:    "",
			NatsJSDomain:        "",
			NatsConnectionName:  "",
			NatsCredentialsFile: "",
			NatsTimeout:         0,
			NatsTLSCert:         "",
			NatsTLSKey:          "",
			NatsTLSCA:           "",
			NatsTLSFirst:        false,
		},
	}

	nc, err := configureNatsConnection(cfg)
	be.NilErr(t, err)

	be.True(t, nc.IsConnected())
}
