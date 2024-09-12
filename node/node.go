package node

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

type Node interface {
	Validate() error
	Start()
}

type nexNode struct {
	nc *nats.Conn

	logger                *slog.Logger
	agentHandshakeTimeout int
	resourceDirectory     string
	internalNodeNATHost   string
	internalNodeNATPort   int
	microVMMode           bool
	preserveNetwork       bool
	kernelFilepath        string
	rootFsFilepath        string
	tags                  map[string]string
	validIssuers          []string
	cniOptions            CNIOptions
	firecrackerOptions    FirecrackerOptions
	bandwidthOptions      BandwithOptions
	operationsOptions     OperationsOptions
	otelOptions           OTelOptions
}

type NexOption func(*nexNode)

func NewNexNode(nc *nats.Conn, opts ...NexOption) (Node, error) {
	if nc == nil {
		return nil, fmt.Errorf("no nats connection provided")
	}

	nn := &nexNode{
		nc:                    nc,
		logger:                slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})),
		agentHandshakeTimeout: 5000,
		resourceDirectory:     "./resources",
		internalNodeNATHost:   nats.DefaultURL,
		internalNodeNATPort:   4222,
		microVMMode:           false,
		preserveNetwork:       true,
		tags:                  make(map[string]string),
		validIssuers:          []string{},
		cniOptions: CNIOptions{
			BinPaths:      []string{"/opt/cni/bin"},
			InterfaceName: "veth0",
			NetworkName:   "fcnet",
			Subnet:        "192.168.127.0/24",
		},
		firecrackerOptions: FirecrackerOptions{
			VcpuCount: 1,
			MemoryMiB: 256,
		},
		bandwidthOptions: BandwithOptions{
			OneTimeBurst: 0,
			RefillTime:   0,
			Size:         0,
		},
		operationsOptions: OperationsOptions{
			OneTimeBurst: 0,
			RefillTime:   0,
			Size:         0,
		},
		otelOptions: OTelOptions{
			MetricsEnabled:   false,
			MetricsPort:      8085,
			MetricsExporter:  "file",
			TracesEnabled:    false,
			TracesExporter:   "file",
			ExporterEndpoint: "127.0.0.1:14532",
		},
	}

	for _, opt := range opts {
		opt(nn)
	}

	return nn, nil
}

func (nn *nexNode) Validate() error {
	var errs error

	if nn.nc == nil {
		errs = errors.Join(errs, errors.New("nats connection is nil"))
	}

	if nn.logger == nil {
		errs = errors.Join(errs, errors.New("logger is nil"))
	}

	if nn.agentHandshakeTimeout <= 0 {
		errs = errors.Join(errs, errors.New("agent handshake timeout must be greater than 0"))
	}

	if nn.resourceDirectory == "" && (nn.kernelFilepath == "" || nn.rootFsFilepath == "") {
		errs = errors.Join(errs, errors.New("must provide a resourceDirectory or a location for kernel and rootfs"))
	} else {
		if nn.resourceDirectory != "" {
			if _, err := os.Stat(nn.resourceDirectory); os.IsNotExist(err) {
				errs = errors.Join(errs, errors.New("resource directory does not exist"))
			}
			if nn.kernelFilepath == "" {
				if _, err := os.Stat(filepath.Join(nn.resourceDirectory, "vmlinux")); os.IsNotExist(err) {
					errs = errors.Join(errs, errors.New("did not find kernel file: "+filepath.Join(nn.resourceDirectory, "vmlinux")))
				}
			}
			if nn.rootFsFilepath == "" {
				if _, err := os.Stat(filepath.Join(nn.resourceDirectory, "rootfs.ext4")); os.IsNotExist(err) {
					errs = errors.Join(errs, errors.New("did not find rootfs file: "+filepath.Join(nn.resourceDirectory, "rootfs.ext4")))
				}
			}
		}
		if nn.kernelFilepath != "" {
			if _, err := os.Stat(nn.kernelFilepath); os.IsNotExist(err) {
				errs = errors.Join(errs, errors.New("kernel file does not exist"))
			}
		}
		if nn.rootFsFilepath != "" {
			if _, err := os.Stat(nn.rootFsFilepath); os.IsNotExist(err) {
				errs = errors.Join(errs, errors.New("rootfs file does not exist"))
			}
		}
	}

	_, err := url.Parse(fmt.Sprintf("%s:%d", nn.internalNodeNATHost, nn.internalNodeNATPort))
	if err != nil {
		errs = errors.Join(errs, errors.New("invalid nats url: "+err.Error()))
	}

	for _, vi := range nn.validIssuers {
		if !nkeys.IsValidPublicServerKey(vi) {
			errs = errors.Join(errs, errors.New("invalid issuer public key: "+vi))
		}
	}

	if nn.otelOptions.MetricsEnabled {
		if nn.otelOptions.MetricsPort <= 0 || nn.otelOptions.MetricsPort > 65535 {
			errs = errors.Join(errs, errors.New("invalid metrics port"))
		}
		if nn.otelOptions.MetricsExporter == "" || !slices.Contains([]string{"file", "prometheus"}, nn.otelOptions.MetricsExporter) {
			errs = errors.Join(errs, errors.New("invalid metrics exporter"))
		}
	}

	if nn.otelOptions.TracesEnabled {
		if nn.otelOptions.TracesExporter == "" || !slices.Contains([]string{"file", "http", "grpc"}, nn.otelOptions.TracesExporter) {
			errs = errors.Join(errs, errors.New("invalid traces exporter"))
		}
		if nn.otelOptions.TracesExporter == "http" || nn.otelOptions.TracesExporter == "grpc" {
			if _, err := url.Parse(nn.otelOptions.ExporterEndpoint); err != nil {
				errs = errors.Join(errs, errors.New("invalid traces exporter endpoint"))
			}
		}
	}

	errs = errors.Join(errs, nn.validateOS())

	return errs
}

func (nn *nexNode) Start() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	<-ctx.Done()
}
