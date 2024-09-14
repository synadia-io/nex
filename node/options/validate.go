package options

import (
	"errors"
	"net/url"
	"os"
	"slices"

	"github.com/nats-io/nkeys"
)

func (opts *NodeOptions) Validate() error {
	var errs error

	if opts.Logger == nil {
		errs = errors.Join(errs, errors.New("logger is nil"))
	}

	if opts.AgentHandshakeTimeout <= 0 {
		errs = errors.Join(errs, errors.New("agent handshake timeout must be greater than 0"))
	}

	if len(opts.WorkloadOptions) <= 0 {
		errs = errors.Join(errs, errors.New("node required at least 1 workload type be configured in order to start"))
	}

	if opts.ResourceDirectory != "" {
		if _, err := os.Stat(opts.ResourceDirectory); os.IsNotExist(err) {
			errs = errors.Join(errs, errors.New("resource directory does not exist"))
		}
	}

	for _, vi := range opts.ValidIssuers {
		if !nkeys.IsValidPublicServerKey(vi) {
			errs = errors.Join(errs, errors.New("invalid issuer public key: "+vi))
		}
	}

	if opts.OtelOptions.MetricsEnabled {
		if opts.OtelOptions.MetricsPort <= 0 || opts.OtelOptions.MetricsPort > 65535 {
			errs = errors.Join(errs, errors.New("invalid metrics port"))
		}
		if opts.OtelOptions.MetricsExporter == "" || !slices.Contains([]string{"file", "prometheus"}, opts.OtelOptions.MetricsExporter) {
			errs = errors.Join(errs, errors.New("invalid metrics exporter"))
		}
	}

	if opts.OtelOptions.TracesEnabled {
		if opts.OtelOptions.TracesExporter == "" || !slices.Contains([]string{"file", "http", "grpc"}, opts.OtelOptions.TracesExporter) {
			errs = errors.Join(errs, errors.New("invalid traces exporter"))
		}
		if opts.OtelOptions.TracesExporter == "http" || opts.OtelOptions.TracesExporter == "grpc" {
			if _, err := url.Parse(opts.OtelOptions.ExporterEndpoint); err != nil {
				errs = errors.Join(errs, errors.New("invalid traces exporter endpoint"))
			}
		}
	}

	return errs
}
