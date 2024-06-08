package nexnode

import (
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	hs "github.com/synadia-io/nex/host-services"
	"github.com/synadia-io/nex/host-services/builtins"
	"github.com/synadia-io/nex/internal/models"
	"go.opentelemetry.io/otel/trace"
)

const hostServiceHTTP = "http"
const hostServiceKeyValue = "kv"
const hostServiceMessaging = "messaging"
const hostServiceObjectStore = "objectstore"

// Host services server implements select functionality which is
// exposed to workloads by way of the agent which makes RPC calls
// via the internal NATS connection
type HostServices struct {
	config *models.HostServicesConfig
	log    *slog.Logger
	ncint  *nats.Conn
	server *hs.HostServicesServer
}

func NewHostServices(
	ncint *nats.Conn,
	config *models.HostServicesConfig,
	log *slog.Logger,
	tracer trace.Tracer,
) *HostServices {
	return &HostServices{
		config: config,
		log:    log,
		ncint:  ncint,
		// ‼️ It cannot be overstated how important it is that the host services server
		// be given the -internal- NATS connection and -not- the external/control one
		//
		// Sincerely,
		//     Someone who lost a day of troubleshooting
		server: hs.NewHostServicesServer(ncint, log, tracer),
	}
}

func (h *HostServices) init() error {
	if httpConfig, ok := h.config.Services[hostServiceHTTP]; ok {
		if httpConfig.Enabled {
			http, err := builtins.NewHTTPService(h.log)
			if err != nil {
				h.log.Error(fmt.Sprintf("failed to initialize http host service: %s", err.Error()))
				return err
			} else {
				h.log.Debug("initialized http host service")
			}

			err = h.server.AddService(hostServiceHTTP, http, httpConfig.Configuration)
			if err != nil {
				return err
			}
		}
	}

	if kvConfig, ok := h.config.Services[hostServiceKeyValue]; ok {
		if kvConfig.Enabled {
			kv, err := builtins.NewKeyValueService(h.log)
			if err != nil {
				h.log.Error(fmt.Sprintf("failed to initialize key/value host service: %s", err.Error()))
				return err
			} else {
				h.log.Debug("initialized key/value host service")
			}

			err = h.server.AddService(hostServiceKeyValue, kv, kvConfig.Configuration)
			if err != nil {
				return err
			}
		}
	}

	if messagingConfig, ok := h.config.Services[hostServiceMessaging]; ok {
		if messagingConfig.Enabled {
			messaging, err := builtins.NewMessagingService(h.log)
			if err != nil {
				h.log.Error(fmt.Sprintf("failed to initialize messaging host service: %s", err.Error()))
				return err
			} else {
				h.log.Debug("initialized messaging host service")
			}

			err = h.server.AddService(hostServiceMessaging, messaging, messagingConfig.Configuration)
			if err != nil {
				return err
			}
		}
	}

	if objectConfig, ok := h.config.Services[hostServiceObjectStore]; ok {
		if objectConfig.Enabled {
			object, err := builtins.NewObjectStoreService(h.log)
			if err != nil {
				h.log.Error(fmt.Sprintf("failed to initialize object store host service: %s", err.Error()))
				return err
			} else {
				h.log.Debug("initialized object store host service")
			}

			err = h.server.AddService(hostServiceObjectStore, object, objectConfig.Configuration)
			if err != nil {
				return err
			}
		}
	}

	h.log.Info("Host services configured", slog.Any("services", h.server.Services()))
	return h.server.Start()
}
