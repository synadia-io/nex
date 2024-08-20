package main

import (
	"context"
	"log/slog"

	"disorder.dev/shandler"
	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/host-services"
	"github.com/synadia-io/nex/host-services/builtins"
	"github.com/synadia-io/nex/host-services/inmem"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nc, err := nats.Connect("0.0.0.0:4222")
	if err != nil {
		panic(err)
	}

	noopProvider := noop.NewTracerProvider()
	otel.SetTracerProvider(noopProvider)
	t := otel.Tracer("")

	var handlerOpts []shandler.HandlerOption
	handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelDebug))
	log := slog.New(shandler.NewHandler(handlerOpts...))

	hsServer := hostservices.NewHostServicesServer(nc, log, t)
	http, _ := builtins.NewHTTPService(log)
	messaging, _ := builtins.NewMessagingService(log)
	kv := inmem.NewInmemKeyValueService(log)
	obj := inmem.NewInmemObjectStore(log)

	hsServer.AddService("http", http, []byte{})
	hsServer.AddService("messaging", messaging, []byte{})
	hsServer.AddService("kv", kv, []byte{})
	hsServer.AddService("objectstore", obj, []byte{})

	_ = hsServer.Start()

	<-ctx.Done()
}
