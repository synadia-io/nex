package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats.go"
)

func generateConnectionFromOpts() (*nats.Conn, error) {
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

	var err error

	exist, _ := fileAccessible(Opts.ConfigurationContext)

	if exist && strings.HasSuffix(Opts.ConfigurationContext, ".json") {
		Opts.Configuration, err = natscontext.NewFromFile(Opts.ConfigurationContext, ctxOpts...)
	} else {
		Opts.Configuration, err = natscontext.New(Opts.ConfigurationContext, !Opts.SkipContexts, ctxOpts...)
	}

	if err != nil {
		return nil, err
	}

	conn, err := Opts.Configuration.Connect()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func fileAccessible(f string) (bool, error) {
	stat, err := os.Stat(f)
	if err != nil {
		return false, err
	}

	if stat.IsDir() {
		return false, fmt.Errorf("is a directory")
	}

	file, err := os.Open(f)
	if err != nil {
		return false, err
	}
	file.Close()

	return true, nil
}
