package nexnode

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"

	nexcli "github.com/ConnectEverything/nex/nex-cli/models"
)

func CmdUp(opts *nexcli.Options, nodeopts *nexcli.NodeOptions, ctx context.Context, cancel context.CancelFunc, log *logrus.Logger) {
	nc, err := generateConnectionFromOpts(opts)
	if err != nil {
		log.WithError(err).Error("Failed to connect to NATS")
		os.Exit(1)
	}

	log.Infof("Established node NATS connection to: %s", opts.Servers)

	config, err := LoadNodeConfiguration(nodeopts.Config)
	if err != nil {
		log.WithError(err).WithField("file", nodeopts.Config).Error("Failed to load node configuration file")
		os.Exit(1)
	}

	log.Infof("Loaded node configuration from '%s'", nodeopts.Config)

	manager, err := NewMachineManager(ctx, cancel, nc, config, log)
	if err != nil {
		log.WithError(err).Error("Failed to initialize machine manager")
		os.Exit(1)
	}

	err = manager.Start()
	if err != nil {
		log.WithError(err).Error("Failed to start machine manager")
		os.Exit(1)
	}

	setupSignalHandlers(log, manager)

	api := NewApiListener(log, manager, config)
	err = api.Start()
	if err != nil {
		log.WithError(err).Error("Failed to start API listener")
		os.Exit(1)
	}
}

func generateConnectionFromOpts(opts *nexcli.Options) (*nats.Conn, error) {
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

func setupSignalHandlers(log *logrus.Logger, manager *MachineManager) {
	go func() {
		// both firecracker and the embedded NATS server register signal handlers... wipe those so ours are the ones being used
		signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGHUP)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt:
				log.Infof("Caught signal: %s, requesting clean shutdown", s.String())
				err := manager.Stop()
				if err != nil {
					log.WithError(err).Warn("Machine manager failed to stop")
				}
				ClearMyApiSockets()
				os.Exit(0)
			case s == syscall.SIGQUIT:
				log.Infof("Caught quit signal: %s, still trying graceful shutdown", s.String())
				err := manager.Stop()
				if err != nil {
					log.WithError(err).Warn("Machine manager failed to stop")
				}
				ClearMyApiSockets()
				os.Exit(0)
			}
		}
	}()
}

func CmdPreflight(opts *nexcli.Options, nodeopts *nexcli.NodeOptions, ctx context.Context, cancel context.CancelFunc, log *logrus.Logger) {
	config, err := LoadNodeConfiguration(nodeopts.Config)
	if err != nil {
		fmt.Printf("Failed to load configuration file: %s\n", err)
		os.Exit(1)
	}

	CheckPreRequisites(config)
}

// TODO : look into also pre-removing /var/lib/cni/networks/fcnet/ during startup sequence
// to ensure we get the full IP range

// Remove firecracker VM sockets created by this pid
func ClearMyApiSockets() {
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
