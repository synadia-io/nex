package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"disorder.dev/shandler"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

func main() {
	var handlerOpts []shandler.HandlerOption
	handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.RFC3339))
	handlerOpts = append(handlerOpts, shandler.WithColor())
	handlerOpts = append(handlerOpts, shandler.WithShortLevels())

	logger := slog.New(shandler.NewHandler(handlerOpts...))

	wi, err := getMetadata()
	if err != nil {
		panic(err)
	}
	nc, err := connectInternalNats()
	if err != nil {
		panic(err)
	}

	runner, err := InitNexExecutionProviderV8(nc, wi, logger)
	if err != nil {
		panic(err)
	}
	err = runner.Validate()
	if err != nil {
		panic(err)
	}
	err = runner.Deploy()
	if err != nil {
		panic(err)
	}
	logger.Info("JavaScript (V8) runner started",
		slog.String("workload_id", wi.VmID),
		slog.String("workload_name", wi.WorkloadName),
		slog.String("namespace", wi.Namespace),
	)
}

func connectInternalNats() (*nats.Conn, error) {
	host := os.Getenv(NexEnvNodeNatsHost)
	port := os.Getenv(NexEnvNodeNatsPort)
	seed := os.Getenv(NexEnvNodeNatsNkeySeed)

	pair, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		return nil, err
	}
	pk, _ := pair.PublicKey()
	natsUrl := fmt.Sprintf("nats://%s:%s", host, port)
	nc, err := nats.Connect(natsUrl, nats.Nkey(pk, func(b []byte) ([]byte, error) {
		return pair.Sign(b)
	}))
	if err != nil {
		return nil, err
	}
	return nc, nil
}

func getMetadata() (*WorkloadInfo, error) {
	if len(os.Args) != 4 {
		return nil, fmt.Errorf("incorrect number of command line arguments: %d", len(os.Args))
	}
	if _, err := os.Stat(os.Args[3]); errors.Is(err, os.ErrNotExist) {
		return nil, errors.Join(errors.New("could not load artifact binary"), err)
	}

	return &WorkloadInfo{
		VmID:         os.Getenv(NexEnvWorkloadId),
		Namespace:    os.Args[1],
		WorkloadName: os.Args[2],
		ArtifactPath: os.Args[3],
	}, nil
}
