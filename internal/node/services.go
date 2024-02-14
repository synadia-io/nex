package nexnode

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/node/services"
	hostservices "github.com/synadia-io/nex/internal/node/services/lib"
)

const hostServiceHTTP = "http"
const hostServiceKeyValue = "kv"
const hostServiceMessaging = "messaging"
const hostServiceObjectStore = "objectstore"

// Host services server implements select functionality which is
// exposed to workloads by way of the agent which makes RPC calls
// via the internal NATS connection
type HostServices struct {
	log   *slog.Logger
	mgr   *MachineManager
	nc    *nats.Conn
	ncint *nats.Conn

	http      services.HostService
	kv        services.HostService
	messaging services.HostService
	object    services.HostService
}

func NewHostServices(mgr *MachineManager, nc, ncint *nats.Conn, log *slog.Logger) *HostServices {
	return &HostServices{
		log:   log,
		mgr:   mgr,
		nc:    nc,
		ncint: ncint,
	}
}

func (h *HostServices) init() error {
	var err error

	h.http, err = hostservices.NewHTTPService(h.nc, h.log)
	if err != nil {
		h.log.Error(fmt.Sprintf("failed to initialize http host service: %s", err.Error()))
		return err
	} else {
		h.log.Debug("initialized http host service")
	}

	h.kv, err = hostservices.NewKeyValueService(h.nc, h.log)
	if err != nil {
		h.log.Error(fmt.Sprintf("failed to initialize key/value host service: %s", err.Error()))
		return err
	} else {
		h.log.Debug("initialized key/value host service")
	}

	h.messaging, err = hostservices.NewMessagingService(h.nc, h.log)
	if err != nil {
		h.log.Error(fmt.Sprintf("failed to initialize messaging host service: %s", err.Error()))
		return err
	} else {
		h.log.Debug("initialized messaging host service")
	}

	h.object, err = hostservices.NewObjectStoreService(h.nc, h.log)
	if err != nil {
		h.log.Error(fmt.Sprintf("failed to initialize object store host service: %s", err.Error()))
		return err
	} else {
		h.log.Debug("initialized object store host service")
	}

	// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
	_, err = h.ncint.Subscribe("agentint.*.rpc.*.*.*.*", h.handleRPC)
	if err != nil {
		return err
	}

	return nil
}

func (h *HostServices) handleRPC(msg *nats.Msg) {
	// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
	tokens := strings.Split(msg.Subject, ".")
	vmID := tokens[1]
	namespace := tokens[3]
	workload := tokens[4]
	service := tokens[5]
	method := tokens[6]

	_, ok := h.mgr.allVMs[vmID]
	if !ok {
		h.log.Warn("Received a host services RPC request from an unknown VM.")
		resp, _ := json.Marshal(map[string]interface{}{
			"error": "unknown vm",
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	h.log.Debug("Received host services RPC request",
		slog.String("vmid", vmID),
		slog.String("namespace", namespace),
		slog.String("workload", workload),
		slog.String("service", service),
		slog.String("method", method),
		slog.Int("payload_size", len(msg.Data)),
	)

	switch service {
	case hostServiceHTTP:
		h.http.HandleRPC(msg)
	case hostServiceKeyValue:
		h.kv.HandleRPC(msg)
	case hostServiceMessaging:
		h.messaging.HandleRPC(msg)
	case hostServiceObjectStore:
		h.object.HandleRPC(msg)
	default:
		h.log.Warn("Received invalid host services RPC request",
			slog.String("service", service),
			slog.String("method", method),
		)

		resp, _ := json.Marshal(map[string]interface{}{
			"error": "invalid rpc request",
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
	}
}
