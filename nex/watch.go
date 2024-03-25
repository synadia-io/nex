package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cdfmlr/ellipsis"
	controlapi "github.com/synadia-io/nex/internal/control-api"
	"github.com/synadia-io/nex/internal/models"
)

func WatchEvents(ctx context.Context, logger *slog.Logger) error {
	nc, err := models.GenerateConnectionFromOpts(Opts, "cli")
	if err != nil {
		return err
	}
	namespaceFilter := "default"
	if len(strings.TrimSpace(Opts.Namespace)) > 0 {
		namespaceFilter = Opts.Namespace
	}

	if strings.Contains(namespaceFilter, "system") {
		return errors.New("namespace filter cannot contain the token 'system', as that is automatically subscribed")
	}

	fmt.Print("\033[H\033[2J")

	logger.Info("Starting event watcher", slog.String("namespace_filter", namespaceFilter))

	apiClient := controlapi.NewApiClient(nc, 1*time.Second, logger)
	eventChannel, err := apiClient.MonitorEvents(namespaceFilter, "*", 0)
	if err != nil {
		return err
	}

	for {
		event := <-eventChannel
		handleEventEntry(logger, event)
	}
}

func WatchLogs(ctx context.Context, logger *slog.Logger) error {
	nc, err := models.GenerateConnectionFromOpts(Opts, "cli")
	if err != nil {
		return err
	}

	nodeFilter := "*"
	if len(strings.TrimSpace(WatchOpts.NodeId)) != 0 {
		nodeFilter = WatchOpts.NodeId
	}
	namespaceFilter := "*"
	if len(strings.TrimSpace(Opts.Namespace)) != 0 {
		namespaceFilter = "default"
	}
	workloadNameFilter := "*"
	if len(strings.TrimSpace(WatchOpts.WorkloadName)) != 0 {
		workloadNameFilter = WatchOpts.WorkloadName
	}
	vmFilter := "*"
	if len(strings.TrimSpace(WatchOpts.WorkloadId)) != 0 {
		vmFilter = WatchOpts.WorkloadId
	}

	fmt.Print("\033[H\033[2J")

	logger.Info("Starting log watcher",
		slog.String("machine_filter", vmFilter),
		slog.String("namespace_filter", namespaceFilter),
		slog.String("node_filter", nodeFilter),
		slog.String("workload_filter", workloadNameFilter),
	)

	apiClient := controlapi.NewApiClient(nc, 1*time.Second, logger)
	ch, err := apiClient.MonitorLogs(namespaceFilter, nodeFilter, workloadNameFilter, vmFilter, 0)
	if err != nil {
		return err
	}
	for {
		logEntry := <-ch
		handleLogEntry(logger, logEntry)
	}
}

func handleEventEntry(log *slog.Logger, emittedEvent controlapi.EmittedEvent) {

	event := emittedEvent.Event
	// TODO: There's likely something we can do to simplify/automate this with reflection or maybe codegen
	attrs := []slog.Attr{slog.String("namespace", emittedEvent.Namespace), slog.String("event_type", emittedEvent.EventType)}

	//	entry := log.WithField("namespace", emittedEvent.Namespace).WithField("event_type", emittedEvent.EventType).WithFields(event.Extensions())

	// Extract meaningful fields from well-known events
	switch emittedEvent.EventType {
	case controlapi.AgentStartedEventType:
		evt := &controlapi.AgentStartedEvent{}
		if err := event.DataAs(evt); err != nil {
			attrs = append(attrs, slog.Any("err", err))
		} else {
			attrs = append(attrs, slog.String("agent_version", evt.AgentVersion))
		}
	case controlapi.AgentStoppedEventType:
		evt := &controlapi.AgentStoppedEvent{}
		if err := event.DataAs(evt); err != nil {
			attrs = append(attrs, slog.Any("err", err))
		} else {
			attrs = append(attrs, slog.String("message", evt.Message), slog.Int("code", evt.Code))
		}
	case controlapi.WorkloadStartedEventType:
		evt := &controlapi.WorkloadStartedEvent{}
		if err := event.DataAs(evt); err != nil {
			attrs = append(attrs, slog.Any("err", err))
		} else {
			attrs = append(attrs, slog.String("workload_name", evt.Name))
		}
	case controlapi.WorkloadStoppedEventType:
		evt := &controlapi.WorkloadStoppedEvent{}
		if err := event.DataAs(evt); err != nil {
			attrs = append(attrs, slog.Any("err", err))
		} else {
			attrs = append(attrs, slog.String("message", evt.Message), slog.Int("code", evt.Code), slog.String("workload_name", evt.Name))
		}
	case controlapi.NodeStartedEventType:
		evt := &controlapi.NodeStartedEvent{}
		if err := event.DataAs(evt); err != nil {
			attrs = append(attrs, slog.Any("err", err))
		} else {
			attrs = append(attrs, slog.String("node_id", ellipsis.Centering(evt.Id, 25)), slog.String("version", evt.Version))
		}
	case controlapi.NodeStoppedEventType:
		evt := &controlapi.NodeStoppedEvent{}
		if err := event.DataAs(evt); err != nil {
			attrs = append(attrs, slog.Any("err", err))
		} else {
			attrs = append(attrs, slog.String("node_id", ellipsis.Centering(evt.Id, 25)), slog.Bool("graceful", evt.Graceful))
		}
	}

	for k, v := range event.Extensions() {
		attrs = append(attrs, slog.Any(k, v))
	}

	log.LogAttrs(context.Background(), slog.LevelInfo, "Received", attrs...)
}

func handleLogEntry(log *slog.Logger, entry controlapi.EmittedLog) {
	log.Log(
		context.Background(),
		slog.LevelDebug,
		entry.Text,
		slog.String("namespace", entry.Namespace),
		slog.String("node", entry.NodeId),
		slog.String("workload", entry.Workload),
		slog.String("vmid", entry.Workload),
	)
}
