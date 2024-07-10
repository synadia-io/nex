package nexnode

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"plugin"
	"runtime"
	"slices"

	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/host-services"
	hs "github.com/synadia-io/nex/host-services"
	"github.com/synadia-io/nex/host-services/builtins"
	"github.com/synadia-io/nex/internal/models"
	"go.opentelemetry.io/otel/trace"
)

const hostServiceHTTP = "http"
const hostServiceKeyValue = "kv"
const hostServiceMessaging = "messaging"
const hostServiceObjectStore = "objectstore"

var hostServicesBuiltins = []string{
	hostServiceHTTP,
	hostServiceKeyValue,
	hostServiceMessaging,
	hostServiceObjectStore,
}

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

	for k, svcConfig := range h.config.Services {
		if !slices.Contains(hostServicesBuiltins, k) {
			if svcConfig.Enabled && svcConfig.PluginPath != nil {
				plug, err := h.loadPlugin(k, svcConfig)
				if err != nil {
					h.log.Error(fmt.Sprintf("failed to load host services plugin %s: %s", k, err))
					return err
				}
				err = h.server.AddService(k, plug, svcConfig.Configuration)
				if err != nil {
					h.log.Error(fmt.Sprintf("failed to add and initialize host service plugin %s: %s", k, err))
				}
			}
		}
	}

	h.log.Info("Host services configured", slog.Any("services", h.server.Services()))
	return h.server.Start()
}

func (h *HostServices) loadPlugin(name string, config models.ServiceConfig) (svc hostservices.HostService, err error) {
	var actualPath string
	defer func() {
		if r := recover(); r != nil {
			err = errors.Join(err, fmt.Errorf("Plugin loader recovered from panic, attempting load of %s", actualPath))
		}
	}()
	actualPath = path.Join(*config.PluginPath, name)

	switch runtime.GOOS {
	case "windows":
		err = errors.New("windows agents do not support host services provider plugins")
		return
	case "linux":
		actualPath = actualPath + ".so"
	case "darwin":
		actualPath = actualPath + ".dylib"
	}

	slog.Debug("Attempting host services plugin load", slog.String("path", actualPath))

	if _, err = os.Stat(actualPath); errors.Is(err, os.ErrNotExist) {
		// path does not exist
		err = fmt.Errorf("plugin path was specified but the file (%s) does not exist", actualPath)
		return
	}

	plug, err := plugin.Open(actualPath)
	if err != nil {
		h.log.Error("failed to open plugin", slog.String("name", name), slog.Any("error", err))
		return
	}

	symPlugin, err := plug.Lookup("HostService")
	if err != nil {
		h.log.Error("failed to look up plugin symbol", slog.Any("error", err))
		return
	}
	var provider hostservices.HostService
	provider, ok := symPlugin.(hostservices.HostService)
	if !ok {
		err = errors.New("unexpected type from module symbol 'HostService'")
		h.log.Error("failed to typecast plugin", slog.String("name", name))
		return
	}

	svc = provider

	return
}
