package nexcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	controlapi "github.com/ConnectEverything/nex/control-api"
	"github.com/cdfmlr/ellipsis"
	"github.com/choria-io/fisk"
	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/nats-io/nats.go"
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

	subscribeSubject := fmt.Sprintf("%s.events.%s.*", controlapi.APIPrefix, namespaceFilter)
	systemSub := fmt.Sprintf("%s.events.system.*", controlapi.APIPrefix) // capture events that come from nodes themselves, e.g. system namespace
	_, err = nc.Subscribe(subscribeSubject, handleEventEntry(log))
	if err != nil {
		return err
	}
	_, err = nc.Subscribe(systemSub, handleEventEntry(log))
	if err != nil {
		return err
	}

	nctx := context.Background()
	<-nctx.Done()

	return nil
}

func WatchLogs(ctx *fisk.ParseContext) error {
	nc, err := generateConnectionFromOpts()
	if err != nil {
		return err
	}

	nodeFilter := "*"
	if len(strings.TrimSpace(WatchOpts.NodeId)) == 0 {
		nodeFilter = WatchOpts.NodeId
	}
	namespaceFilter := "*"
	if len(strings.TrimSpace(Opts.Namespace)) == 0 {
		namespaceFilter = "default"
	}
	workloadNameFilter := "*"
	if len(strings.TrimSpace(WatchOpts.WorkloadName)) == 0 {
		workloadNameFilter = WatchOpts.WorkloadName
	}
	vmFilter := "*"
	if len(strings.TrimSpace(WatchOpts.WorkloadId)) == 0 {
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

	// $NEX.logs.{namespace}.{node}.{workload name}.{vm}
	subscribeSubject := fmt.Sprintf("%s.logs.%s.%s.%s.%s", controlapi.APIPrefix, namespaceFilter, nodeFilter, workloadNameFilter, vmFilter)
	_, err = nc.Subscribe(subscribeSubject, handleLogEntry(log))
	if err != nil {
		return err
	}

	nctx := context.Background()
	<-nctx.Done()

	return nil
}

func handleEventEntry(log *logrus.Logger) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		tokens := strings.Split(m.Subject, ".")
		if len(tokens) != 4 {
			return
		}

		namespace := tokens[2]
		eventType := tokens[3]

		event := cloudevents.NewEvent()
		err := json.Unmarshal(m.Data, &event)
		if err != nil {
			return
		}

		// TODO: There's likely something we can do to simplify/automate this with reflection or maybe codegen
		entry := log.WithField("namespace", namespace).WithField("event_type", eventType).WithFields(event.Extensions())
		switch eventType {
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
}

func handleLogEntry(log *logrus.Logger) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		tokens := strings.Split(m.Subject, ".")
		if len(tokens) != 6 {
			return
		}
		namespace := tokens[2]
		// missed opportunity to call this method "MiddleOut"
		node := ellipsis.Centering(tokens[3], 25)
		workload := tokens[4]
		vm := tokens[5]

		var logEntry emittedLog
		err := json.Unmarshal(m.Data, &logEntry)
		if err != nil {
			log.WithError(err).Error("Log entry deserialization failure")
			return
		}

		log.WithField("namespace", namespace).
			WithField("node", node).
			WithField("workload", workload).
			WithField("vmid", vm).
			Log(log.Level, logEntry.Text)
	}
}

type emittedLog struct {
	Text      string       `json:"text"`
	Level     logrus.Level `json:"level"`
	MachineId string       `json:"machine_id"`
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
