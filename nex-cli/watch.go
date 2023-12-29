package nexcli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	controlapi "github.com/ConnectEverything/nex/control-api"
	"github.com/cdfmlr/ellipsis"
	"github.com/choria-io/fisk"
	"github.com/sirupsen/logrus"
)

func WatchEvents(ctx *fisk.ParseContext) error {
	nc, err := generateConnectionFromOpts()
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
	log := buildWatcherLog()
	log.
		WithField("namespace_filter", namespaceFilter).
		// FIX: 👀 in the info string upsets the formatter
		Info("Starting event watcher")

	apiClient := controlapi.NewApiClient(nc, 1*time.Second, log)
	eventChannel, err := apiClient.MonitorEvents(namespaceFilter, "*", 0)
	if err != nil {
		return err
	}

	for {
		event := <-eventChannel
		handleEventEntry(log, event)
	}
}

func WatchLogs(ctx *fisk.ParseContext) error {
	nc, err := generateConnectionFromOpts()
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
	log := buildWatcherLog()

	log.
		WithField("node_filter", nodeFilter).
		WithField("namespace_filter", namespaceFilter).
		WithField("workload_filter", workloadNameFilter).
		WithField("machine_filter", vmFilter).
		// FIX: 👀 in this text upsets the formatter. Emoji usage is priority
		Info("Starting log watcher")

	apiClient := controlapi.NewApiClient(nc, 1*time.Second, log)
	ch, err := apiClient.MonitorLogs(namespaceFilter, nodeFilter, workloadNameFilter, vmFilter, 0)
	if err != nil {
		return err
	}
	for {
		logEntry := <-ch
		handleLogEntry(log, logEntry)
	}
}

func handleEventEntry(log *logrus.Logger, emittedEvent controlapi.EmittedEvent) {

	event := emittedEvent.Event
	// TODO: There's likely something we can do to simplify/automate this with reflection or maybe codegen
	entry := log.WithField("namespace", emittedEvent.Namespace).WithField("event_type", emittedEvent.EventType).WithFields(event.Extensions())

	// Extract meaningful fields from well-known events
	switch emittedEvent.EventType {
	case controlapi.AgentStartedEventType:
		evt := &controlapi.AgentStartedEvent{}
		if err := event.DataAs(evt); err != nil {
			entry = entry.WithError(err)
		} else {
			entry = entry.WithField("agent_version", evt.AgentVersion)
		}
	case controlapi.AgentStoppedEventType:
		evt := &controlapi.AgentStoppedEvent{}
		if err := event.DataAs(evt); err != nil {
			entry = entry.WithError(err)
		} else {
			entry = entry.WithField("message", evt.Message).WithField("code", evt.Code)
		}
	case controlapi.WorkloadStartedEventType:
		evt := &controlapi.WorkloadStartedEvent{}
		if err := event.DataAs(evt); err != nil {
			entry = entry.WithError(err)
		} else {
			entry = entry.WithField("workload_name", evt.Name)
		}
	case controlapi.WorkloadStoppedEventType:
		evt := &controlapi.WorkloadStoppedEvent{}
		if err := event.DataAs(evt); err != nil {
			entry = entry.WithError(err)
		} else {
			entry = entry.WithField("message", evt.Message).WithField("code", evt.Code).WithField("workload_name", evt.Name)
		}
	case controlapi.NodeStartedEventType:
		evt := &controlapi.NodeStartedEvent{}
		if err := event.DataAs(evt); err != nil {
			entry = entry.WithError(err)
		} else {
			entry = entry.WithField("node_id", ellipsis.Centering(evt.Id, 25)).WithField("version", evt.Version)
		}
	case controlapi.NodeStoppedEventType:
		evt := &controlapi.NodeStoppedEvent{}
		if err := event.DataAs(evt); err != nil {
			entry = entry.WithError(err)
		} else {
			entry = entry.WithField("node_id", ellipsis.Centering(evt.Id, 25)).WithField("graceful", evt.Graceful)
		}
	}

	entry.Info("Received")
}

func handleLogEntry(log *logrus.Logger, entry controlapi.EmittedLog) {
	if entry.Level == 0 {
		entry.Level = logrus.DebugLevel
	}

	log.WithField("namespace", entry.Namespace).
		WithField("node", entry.NodeId).
		WithField("workload", entry.Workload).
		WithField("vmid", entry.Workload).
		Log(log.Level, entry.Text)
}

func buildWatcherLog() *logrus.Logger {
	log := logrus.New()

	log.SetFormatter(&logrus.TextFormatter{
		ForceColors:      true,
		DisableTimestamp: false,
		FullTimestamp:    true,
		QuoteEmptyFields: true,
		TimestampFormat:  "2006-01-02 15:04:05",
	})
	log.SetReportCaller(false)
	lvl := WatchOpts.LogLevel

	if len(strings.TrimSpace(lvl)) == 0 {
		lvl = "debug"
	}
	ll, err := logrus.ParseLevel(lvl)
	if err != nil {
		ll = logrus.DebugLevel
	}
	// set global log level
	log.SetLevel(ll)
	return log
}
