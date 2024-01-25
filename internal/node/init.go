package nexnode

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats.go"

	nexmodels "github.com/synadia-io/nex/internal/models"
)

func CmdUp(opts *nexmodels.Options, nodeopts *nexmodels.NodeOptions, ctx context.Context, cancel context.CancelFunc, log *slog.Logger) {
	nc, err := generateConnectionFromOpts(opts)
	if err != nil {
		log.Error("Failed to connect to NATS server", slog.Any("err", err))
		panic("failed to connect to NATS server")
	}

	log.Info("Established node NATS connection", slog.String("servers", opts.Servers))

	config, err := LoadNodeConfiguration(nodeopts.ConfigFilepath)
	if err != nil {
		log.Error("Failed to load node configuration file", slog.Any("err", err), slog.String("config_path", nodeopts.ConfigFilepath))
		panic("failed to load node configuration file")
	}

	log.Info("Loaded node configuration from '%s'", slog.String("config_path", nodeopts.ConfigFilepath))

	manager, err := NewMachineManager(ctx, cancel, nc, config, log)
	if err != nil {
		log.Error("Failed to initialize machine manager", slog.Any("err", err))
		panic("failed to initialize machine manager")
	}

	err = manager.Start()
	if err != nil {
		log.Error("Failed to start machine manager", slog.Any("err", err))
		panic("failed to start machine manager")
	}

	setupSignalHandlers(log, manager)

	api := NewApiListener(log, manager, config)
	err = api.Start()
	if err != nil {
		log.Error("Failed to start API listener", slog.Any("err", err))
		panic("failed to start API listener")
	}
}

// TODO
// FIXME
// if I export this to the `nex` binary, then macOS won't build.
// why? what did I do to deserve this?
func generateConnectionFromOpts(opts *nexmodels.Options) (*nats.Conn, error) {
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

	var err error

	exist, _ := fileAccessible(opts.ConfigurationContext)

	if exist && strings.HasSuffix(opts.ConfigurationContext, ".json") {
		opts.Configuration, err = natscontext.NewFromFile(opts.ConfigurationContext, ctxOpts...)
	} else {
		opts.Configuration, err = natscontext.New(opts.ConfigurationContext, !opts.SkipContexts, ctxOpts...)
	}

	if err != nil {
		return nil, err
	}

	conn, err := opts.Configuration.Connect()
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

func setupSignalHandlers(log *slog.Logger, manager *MachineManager) {
	go func() {
		// both firecracker and the embedded NATS server register signal handlers... wipe those so ours are the ones being used
		signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGHUP)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt:
				log.Info("Caught signal, requesting clean shutdown", slog.String("signal", s.String()))
				err := manager.Stop()
				if err != nil {
					log.Warn("Machine manager failed to stop", slog.Any("err", err))
				}
				cleanSockets(log)
				os.Exit(0) // FIXME
			case s == syscall.SIGQUIT:
				log.Info("Caught quit signal, still trying graceful shutdown", slog.String("signal", s.String()))
				err := manager.Stop()
				if err != nil {
					log.Warn("Machine manager failed to stop", slog.Any("err", err))
				}
				cleanSockets(log)
				os.Exit(0) // FIXME
			}
		}
	}()
}

func CmdPreflight(opts *nexmodels.Options, nodeopts *nexmodels.NodeOptions, ctx context.Context, cancel context.CancelFunc, log *slog.Logger) {
	config, err := LoadNodeConfiguration(nodeopts.ConfigFilepath)
	if err != nil {
		panic(fmt.Errorf("failed to load configuration file: %s", err))
	}

	config.ForceDepInstall = nodeopts.ForceDepInstall

	err = CheckPreRequisites(config)
	if err != nil {
		panic(fmt.Errorf("preflight checks failed: %s", err))
	}
}

// TODO : look into also pre-removing /var/lib/cni/networks/fcnet/ during startup sequence
// to ensure we get the full IP range

// Remove firecracker VM sockets created by this pid
func cleanSockets(logger *slog.Logger) {
	dir, err := os.ReadDir(os.TempDir())
	if err != nil {
		logger.Error("Failed to read temp directory", slog.Any("err", err))
	}
	for _, d := range dir {
		if strings.Contains(d.Name(), fmt.Sprintf(".firecracker.sock-%d-", os.Getpid())) {
			os.Remove(path.Join([]string{"tmp", d.Name()}...))
		}
	}
}
