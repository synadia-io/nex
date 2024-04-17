[![Lint | Test | Build](https://github.com/synadia-io/nex/actions/workflows/ltb.yml/badge.svg)](https://github.com/synadia-io/nex/actions/workflows/ltb.yml)
![Release](https://github.com/synadia-io/nex/actions/workflows/release.yml/badge.svg)
![Homepage and Documentation](https://img.shields.io/website?label=Homepage&url=https%3A%2F%2Fnats.io)
![eg](https://img.shields.io/badge/Powered%20By-NATS-green)
![GitHub License](https://img.shields.io/github/license/synadia-io/nex)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/synadia-io/nex)
[![Go Report Card](https://goreportcard.com/badge/github.com/synadia-io/nex)](https://goreportcard.com/report/github.com/synadia-io/nex)
[![Go Reference](https://pkg.go.dev/badge/github.com/synadia-io/nex.svg)](https://pkg.go.dev/github.com/synadia-io/nex)

# NATS Execution Engine
Leverage and extend your investment in NATS infrastructure to deploy functions and services, turning NATS into the ultimate platform for building distributed applications.

## Quickstart
The easiest way to get started with Nex is to check out our [Using Nex](https://docs.nats.io/using-nats/nex) section in the NATS documentation.

If you're already familiar with Nex and how it works, then you can get going quickly by installing the Nex CLI:

```
curl -sSf https://nex.synadia.com/install.sh | sh 
```

## Preflight Check

If you haven't already, please make sure that `/usr/local/bin` is in your path. 

The `nex node preflight` command is here to help you bootstrap your system and ensure that you have all of the elements you need to start and operate Nex nodes.

The pre-flight check requires a configuration file so it can perform the appropriate checks. If you don't already have a configuration file, then Nex's `preflight` command will generate a new one for you in `./config.json`, as shown below:

```json
{
    "default_resource_dir":"/tmp/wd",
    "machine_pool_size": 1,
    "cni": {
        "network_name": "fcnet",
        "interface_name": "veth0"
    },
    "machine_template": {
        "vcpu_count": 1,
        "memsize_mib": 256
    },
    "tags": {
        "simple": "true"
    },
    "no_sandbox": false,
    "otlp_exporter_url": null
}
```

Run the following command to get everything set up using the above defaults:

```
$ nex node preflight
```

## Nex Components
Nex is made up of the following components

* [agent](./agent) - Agent that runs inside a Firecracker VM, responsible for running untrusted workloads. Not something end users need to interact with.
* [node](./internal/node) - Service running on a NEX node. Exposes a control API, starts/stops firecracker processes, communicates with the agent inside each process.
* [nex](./nex) - CLI for communicating with NEX nodes
* [tui](./nex/tui) - Interactive user interface for viewing the status of NEX nodes in a terminal. BETA feature, not yet fully functional.

## Telemetry Data
Nex comes prewired with [OpenTelemetry](https://opentelemetry.io) support for traces and metrics.  

In order to enable OTel support, you will first need to provide the Nex node with access to an OTel exporter.  We have provided a docker-compose solution for local development.  In order to start the OTel evironment, you will need navigate to the `_scripts/otel` directory and run `docker compose up`.  The following ports will be exposed:
```
Web UI (grafana)      -> :14524
OTel Collector (grpc) -> :24317
OTel Collector (http) -> :24318
```

In order to view the metrics and traces, you will need to navigate to `http://localhost:14524` in your browser.

### Traces
To enable traces, add `"otlp_exporter_url": "0.0.0.0:24317"` to the bottom of your Nex configuration file
```bash
nex node up \
  --traces \                      # enables traces
  --otel_traces_exporter grpc     # controls where how traces are exported to collector
```

### Metrics
To enable metrics, include the flags when starting the node
```bash
nex node up \
  --metrics \                      # enables metrics
  --otel_metrics_exporter stdout \ # controls whether the metrics are printed to stdout or provided via prometheus
  --metrics_port 8085              # exposes prometheus data on provided port
```


## Contributing
For information on how to contribute to Nex, please read our [contributing](./CONTRIBUTING.md) guide.
