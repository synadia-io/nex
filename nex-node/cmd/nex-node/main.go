//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	nexnode "github.com/ConnectEverything/nex/nex-node"
	"github.com/choria-io/fisk"
	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

const (
	help = `NEX Node Service

Service that manages a NATS Execution Engine node

`
)

func main() {
	defer clearMyApiSockets()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logrus.New()
	log.SetReportCaller(false)

	lvl, ok := os.LookupEnv("LOG_LEVEL")

	if !ok || len(strings.TrimSpace(lvl)) == 0 {
		lvl = "debug"
	}
	ll, err := logrus.ParseLevel(lvl)
	if err != nil {
		ll = logrus.DebugLevel
	}
	// set global log level
	log.SetLevel(ll)

	opts := &nexnode.CliOptions{}

	ncli := fisk.New("nex-node", help)
	ncli.Author("Synadia Communications")
	ncli.UsageWriter(os.Stdout)
	ncli.Version(nexnode.VERSION)
	ncli.HelpFlag.Short('h')
	ncli.WithCheats().CheatCommand.Hidden()

	ncli.Flag("server", "NATS server urls").Short('s').Envar("NATS_URL").PlaceHolder("URL").StringVar(&opts.Servers)
	ncli.Flag("user", "Username or Token").Envar("NATS_USER").PlaceHolder("USER").StringVar(&opts.Username)
	ncli.Flag("password", "Password").Envar("NATS_PASSWORD").PlaceHolder("PASSWORD").StringVar(&opts.Password)
	ncli.Flag("creds", "User credentials file (JWT authentication)").Envar("NATS_CREDS").PlaceHolder("FILE").StringVar(&opts.Creds)
	ncli.Flag("nkey", "User NKEY file for single-key auth").Envar("NATS_NKEY").PlaceHolder("FILE").StringVar(&opts.Nkey)
	ncli.Flag("tlscert", "TLS public certificate file").Envar("NATS_CERT").PlaceHolder("FILE").ExistingFileVar(&opts.TlsCert)
	ncli.Flag("tlskey", "TLS private key file").Envar("NATS_KEY").PlaceHolder("FILE").ExistingFileVar(&opts.TlsKey)
	ncli.Flag("tlsca", "TLS certificate authority chain file").Envar("NATS_CA").PlaceHolder("FILE").ExistingFileVar(&opts.TlsCA)
	ncli.Flag("tlsfirst", "Perform TLS handshake before expecting the server greeting").BoolVar(&opts.TlsFirst)
	ncli.Flag("timeout", "Time to wait on responses from NATS").Default("5s").Envar("NATS_TIMEOUT").PlaceHolder("DURATION").DurationVar(&opts.Timeout)
	ncli.Flag("config", "Path to the node configuration file").Required().ExistingFileVar(&opts.NodeConfigFile)

	ncli.Command("up", "Starts the node execution engine")
	ncli.Command("preflight", "Examines the environment to verify pre-requisites")

	cmd := ncli.MustParseWithUsage(os.Args[1:])
	opts.ConnectionName = "nex-node " + nexnode.VERSION
	switch cmd {
	case "up":
		cmdUp(opts, ctx, cancel, log)
		<-ctx.Done()
	case "preflight":
		cmdPreflight(opts, ctx, cancel, log)
	}
}

func cmdUp(opts *nexnode.CliOptions, ctx context.Context, cancel context.CancelFunc, log *logrus.Logger) {

	nc, err := generateConnectionFromOpts(opts)
	if err != nil {
		log.WithError(err).Error("Failed to connect to NATS")
		os.Exit(1)
	}

	log.Infof("Established node NATS connection to: %s", opts.Servers)

	config, err := nexnode.LoadNodeConfiguration(opts.NodeConfigFile)
	if err != nil {
		log.WithError(err).WithField("file", opts.NodeConfigFile).Error("Failed to load node configuration file")
		os.Exit(1)
	}

	log.Infof("Loaded node configuration from '%s'", opts.NodeConfigFile)

	manager := nexnode.NewMachineManager(ctx, cancel, nc, config, log)
	err = manager.Start()
	if err != nil {
		log.WithError(err).Error("Failed to start machine manager")
		os.Exit(1)
	}

	setupSignalHandlers(log, manager)

	api := nexnode.NewApiListener(log, manager, make(map[string]string))
	api.Start()
}

func cmdPreflight(opts *nexnode.CliOptions, ctx context.Context, cancel context.CancelFunc, log *logrus.Logger) {
	config, err := nexnode.LoadNodeConfiguration(opts.NodeConfigFile)
	if err != nil {
		fmt.Printf("Failed to load configuration file: %s\n", err)
		os.Exit(1)
	}

	nexnode.CheckPreRequisites(config)
}

// TODO : look into also pre-removing /var/lib/cni/networks/fcnet/ during startup sequence
// to ensure we get the full IP range

// Remove firecracker VM sockets created by this pid
func clearMyApiSockets() {
	dir, err := os.ReadDir(os.TempDir())
	if err != nil {
		logrus.WithError(err).Error("Failed to read temp directory")
	}
	for _, d := range dir {
		if strings.Contains(d.Name(), fmt.Sprintf(".firecracker.sock-%d-", os.Getpid())) {
			os.Remove(path.Join([]string{"tmp", d.Name()}...))
		}
	}
}

func setupSignalHandlers(log *logrus.Logger, manager *nexnode.MachineManager) {
	go func() {
		// both firecracker and the embedded NATS server register signal handlers... wipe those so ours are the ones being used
		signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGHUP)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt:
				log.Infof("Caught signal: %s, requesting clean shutdown", s.String())
				manager.Stop()
				clearMyApiSockets()
				os.Exit(0)
			case s == syscall.SIGQUIT:
				log.Infof("Caught quit signal: %s, still trying graceful shutdown", s.String())
				manager.Stop()
				clearMyApiSockets()
				os.Exit(0)
			}
		}
	}()
}

func generateConnectionFromOpts(opts *nexnode.CliOptions) (*nats.Conn, error) {
	if len(strings.TrimSpace(opts.Servers)) == 0 {
		opts.Servers = nats.DefaultURL
	}
	ctxOpts := []natscontext.Option{
		natscontext.WithServerURL(opts.Servers),
		natscontext.WithCreds(opts.Creds),
		natscontext.WithNKey(opts.Nkey),
		natscontext.WithCertificate(opts.TlsCert),
		natscontext.WithKey(opts.TlsKey),
		natscontext.WithCA(opts.TlsCA),
	}

	if opts.TlsFirst {
		ctxOpts = append(ctxOpts, natscontext.WithTLSHandshakeFirst())
	}

	if opts.Username != "" && opts.Password == "" {
		ctxOpts = append(ctxOpts, natscontext.WithToken(opts.Username))
	} else {
		ctxOpts = append(ctxOpts, natscontext.WithUser(opts.Username), natscontext.WithPassword(opts.Password))
	}

	natsContext, err := natscontext.New("nexnode", false, ctxOpts...)

	if err != nil {
		return nil, err
	}

	natsOpts, err := natsContext.NATSOptions()
	if err != nil {
		return nil, err
	}

	conn, err := nats.Connect(opts.Servers, natsOpts...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
