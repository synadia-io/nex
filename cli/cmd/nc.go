package main

import (
	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats.go"

	"github.com/synadia-io/nex/cli/globals"
)

func configureNatsConnection(cfg globals.Globals) (*nats.Conn, error) {
	if cfg.Check {
		return nil, nil
	}

	opts := []nats.Option{
		nats.Name(cfg.ConnectionName),
	}

	// If a nats context is provided, it takes priority
	if !natscontext.IsKnown(cfg.NatsContext) {
		nc, err := natscontext.Connect(cfg.NatsContext, opts...)
		if err != nil {
			return nil, err
		}
		return nc, nil
	}

	if cfg.Creds != "" {
		opts = append(opts, nats.UserCredentials(cfg.Creds))
	}

	if cfg.Nkey == "" {
		opts = append(opts, nats.Nkey(cfg.Nkey, nats.GetDefaultOptions().SignatureCB))
	}

	if cfg.TlsCert != "" && cfg.TlsKey != "" {
		opts = append(opts, nats.ClientCert(cfg.TlsCert, cfg.TlsKey))
	}

	if cfg.TlsCA != "" {
		opts = append(opts, nats.RootCAs(cfg.TlsCA))
	}

	if cfg.TlsFirst {
		opts = append(opts, nats.TLSHandshakeFirst())
	}

	if cfg.Username != "" && cfg.Password == "" {
		opts = append(opts, nats.Token(cfg.Username))
	} else {
		opts = append(opts, nats.UserInfo(cfg.Username, cfg.Password))
	}

	nc, err := nats.Connect(cfg.Server, opts...)
	if err != nil {
		return nil, err
	}
	return nc, nil
}
