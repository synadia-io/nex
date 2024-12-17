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

## Prerequisites

### Taskfile

Nex uses [Task](https://taskfile.dev) for building, testing, and releasing. You can install Task by following the
instructions [here](https://taskfile.dev/installation)

### Protobuf | Go-jsonschema

Nex relies on Protobuf and Go-jsonschema for defining the API and messages. While many linux distros have Protobuf available in their
package repositories, it is recommended to install the version matching what is defined in CI from the official releases. You can find the
latest release [here](https://github.com/protocolbuffers/protobuf/releases)

The included Taskfile has a task defined for downloading and installing the required dependencies.

```shell
task install-deps
```

## Getting Started

Currently, there are no pre-built binaries available. To build from source, make sure the prerequisites are installed.
Once you got these dependencies installed and this repository cloned, you can build the `nex` binary with the
following command:

```shell
task nex
```

With the nex binary built and a local nats server running, we can finally run our node:

```shell
mkdir -p ~/.config/nex
target/nex node up
```
