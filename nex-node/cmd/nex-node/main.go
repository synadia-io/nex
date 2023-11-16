package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	nex "github.com/ConnectEverything/nex/nex-node"
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

	opts := &nex.CliOptions{}

	ncli := fisk.New("nex-node", help)
	ncli.Author("Synadia Communications")
	ncli.UsageWriter(os.Stdout)
	ncli.Version(nex.VERSION)
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
	ncli.Flag("machineconfig", "Path to the machine management configuration file").Required().ExistingFileVar(&opts.MachineConfigFile)

	ncli.Command("up", "Starts the node execution engine")
	ncli.Command("preflight", "Examines the environment to verify pre-requisites")

	cmd := ncli.MustParseWithUsage(os.Args[1:])
	opts.ConnectionName = "nex-node " + nex.VERSION
	switch cmd {
	case "up":
		cmdUp(opts, ctx, cancel, log)
	case "preflight":
		cmdPreflight(opts, ctx, cancel, log)
	}

	<-ctx.Done()
}

func cmdUp(opts *nexnode.CliOptions, ctx context.Context, cancel context.CancelFunc, log *logrus.Logger) {

	nc, err := generateConnectionFromOpts(opts)
	if err != nil {
		log.WithError(err).Error("Failed to connect to NATS")
		os.Exit(1)
	}

	log.Infof("Established node NATS connection to: %s", opts.Servers)

	config, err := nex.LoadNodeConfiguration(opts.MachineConfigFile)
	if err != nil {
		log.WithError(err).WithField("file", opts.MachineConfigFile).Error("Failed to load machine configuration file")
		os.Exit(1)
	}

	log.Infof("Loaded machine management configuration from '%s'", opts.MachineConfigFile)

	manager := nex.NewMachineManager(ctx, cancel, nc, config, log)
	setupSignalHandlers(log, manager)
	err = manager.Start()
	if err != nil {
		panic(err)
	}
	nodeStart := struct {
		Version string `json:"version"`
		Id      string `json:"id"`
	}{
		Version: nex.VERSION,
		Id:      manager.PublicKey(),
	}
	manager.PublishCloudEvent("node_started", nodeStart)

	api := nex.NewApiListener(nc, log, manager, make(map[string]string))
	api.Start()
}

func cmdPreflight(opts *nexnode.CliOptions, ctx context.Context, cancel context.CancelFunc, log *logrus.Logger) {
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

func setupSignalHandlers(log *logrus.Logger, manager *nex.MachineManager) {
	go func() {
		// Clear some default handlers installed by the firecracker SDK:
		signal.Reset(os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt:
				log.Infof("Caught signal: %s, requesting clean shutdown", s.String())
				manager.Stop()
				clearMyApiSockets()
				os.Exit(0)
			case s == syscall.SIGQUIT:
				log.Infof("Caught signal: %s, forcing shutdown", s.String())
				manager.Stop()
				clearMyApiSockets()
				os.Exit(0)
			}
		}
	}()
}

func generateConnectionFromOpts(opts *nex.CliOptions) (*nats.Conn, error) {
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
