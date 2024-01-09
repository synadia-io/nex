package main

import (
	"strings"

	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats.go"
)

func generateConnectionFromOpts() (*nats.Conn, error) {
	if len(strings.TrimSpace(Opts.Servers)) == 0 {
		Opts.Servers = "0.0.0.0:4222"
	}
	ctxOpts := []natscontext.Option{
		natscontext.WithServerURL(Opts.Servers),
		natscontext.WithCreds(Opts.Creds),
		natscontext.WithNKey(Opts.Nkey),
		natscontext.WithCertificate(Opts.TlsCert),
		natscontext.WithKey(Opts.TlsKey),
		natscontext.WithCA(Opts.TlsCA),
	}

	if Opts.TlsFirst {
		ctxOpts = append(ctxOpts, natscontext.WithTLSHandshakeFirst())
	}

	if Opts.Username != "" && Opts.Password == "" {
		ctxOpts = append(ctxOpts, natscontext.WithToken(Opts.Username))
	} else {
		ctxOpts = append(ctxOpts, natscontext.WithUser(Opts.Username), natscontext.WithPassword(Opts.Password))
	}

	natsContext, err := natscontext.New("nexnode", false, ctxOpts...)

	if err != nil {
		return nil, err
	}

	natsOpts, err := natsContext.NATSOptions()
	if err != nil {
		return nil, err
	}

	conn, err := nats.Connect(Opts.Servers, natsOpts...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
