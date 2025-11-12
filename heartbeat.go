package nex

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/synadia-io/nex/models"
)

type hb struct {
	Registrations string `json:"registrations"`
}

func (n *NexNode) heartbeat() {
	pubKey, err := n.nodeKeypair.PublicKey()
	if err != nil {
		n.logger.Error("failed to get public key for heartbeat", slog.String("err", err.Error()))
		return
	}

	for range time.Tick(10 * time.Second) {
		if n.nc.IsClosed() {
			return
		}

		beat := hb{
			Registrations: n.registeredAgents.String(),
		}

		hbB, err := json.Marshal(beat)
		if err != nil {
			n.logger.Error("failed to Marshal heartbeat", slog.String("err", err.Error()))
		}

		err = n.nc.Publish(models.NodeEmitHeartbeatSubject(pubKey), hbB)
		if err != nil {
			n.logger.Error("failed to publish heartbeat", slog.String("err", err.Error()))
		}
	}
}
