package actors

import (
	"log/slog"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
)

func TestGenerateConfig(t *testing.T) {
	hostUser, err := nkeys.CreateUser()
	be.NilErr(t, err)

	ns := &InternalNatsServer{
		hostUser: hostUser,
		logger:   slog.Default(),
		devMode:  true,
		nodeOptions: models.NodeOptions{
			AgentOptions: []models.AgentOptions{
				{
					Name: "native",
				},
				{
					Name: "javascript",
				},
			},
		},
	}

	ns.creds, err = ns.buildAgentCredentials()
	be.NilErr(t, err)
	be.Equal(t, 2, len(ns.creds))

	opts, err := ns.generateConfig()
	be.NilErr(t, err)
	be.True(t, opts != nil)
	be.True(t, opts.Accounts != nil)
	be.Equal(t, 3, len(opts.Accounts))
}
