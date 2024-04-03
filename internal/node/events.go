package nexnode

import (
	"fmt"
	"log/slog"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/nats-io/nats.go"
)

// FIXME-- move this to types repo-- audit other places where it is redeclared (nex-cli)
type emittedLog struct {
	Text  string     `json:"text"`
	Level slog.Level `json:"level"`
	ID    string     `json:"id"`
}

// publish the given $NEX event to an arbitrary namespace using the given NATS connection
func PublishCloudEvent(nc *nats.Conn, namespace string, event cloudevents.Event, log *slog.Logger) error {
	raw, _ := event.MarshalJSON()

	// $NEX.events.{namespace}.{event_type}
	subject := fmt.Sprintf("%s.%s.%s", EventSubjectPrefix, namespace, event.Type())
	err := nc.Publish(subject, raw)
	if err != nil {
		log.Error("Failed to publish cloud event", slog.Any("err", err))
		return err
	}

	return nc.Flush()
}
