package nexnode

import (
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	hs "github.com/synadia-io/nex/host-services"
	"github.com/synadia-io/nex/host-services/builtins"
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
	mgr   *WorkloadManager
	nc    *nats.Conn
	ncint *nats.Conn

	hsServer *hs.HostServicesServer
}

func NewHostServices(mgr *WorkloadManager, nc, ncint *nats.Conn, log *slog.Logger) *HostServices {
	return &HostServices{
		log:      log,
		mgr:      mgr,
		nc:       nc,
		ncint:    ncint,
		hsServer: hs.NewHostServicesServer(nc),
	}
}

func (h *HostServices) init() error {
	var err error

	http, err := builtins.NewHTTPService(h.nc, h.log)
	if err != nil {
		h.log.Error(fmt.Sprintf("failed to initialize http host service: %s", err.Error()))
		return err
	} else {
		h.log.Debug("initialized http host service")
	}
	h.hsServer.AddService(hostServiceHTTP, http, make(map[string]string))

	kv, err := builtins.NewKeyValueService(h.nc, h.log)
	if err != nil {
		h.log.Error(fmt.Sprintf("failed to initialize key/value host service: %s", err.Error()))
		return err
	} else {
		h.log.Debug("initialized key/value host service")
	}
	h.hsServer.AddService(hostServiceKeyValue, kv, make(map[string]string))

	messaging, err := builtins.NewMessagingService(h.nc, h.log)
	if err != nil {
		h.log.Error(fmt.Sprintf("failed to initialize messaging host service: %s", err.Error()))
		return err
	} else {
		h.log.Debug("initialized messaging host service")
	}
	h.hsServer.AddService(hostServiceMessaging, messaging, make(map[string]string))

	object, err := builtins.NewObjectStoreService(h.nc, h.log)
	if err != nil {
		h.log.Error(fmt.Sprintf("failed to initialize object store host service: %s", err.Error()))
		return err
	} else {
		h.log.Debug("initialized object store host service")
	}
	h.hsServer.AddService(hostServiceObjectStore, object, make(map[string]string))

	return h.hsServer.Start()
}
