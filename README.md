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

### Protobuf
Nex relies on Protobuf for defining the API and messages. While many linux distros have Protobuf available in their 
package repositories, it is recommended to install the latest version from the official releases. You can find the 
latest release [here](https://github.com/protocolbuffers/protobuf/releases)

On linux, once downloaded, you can extract the archive and run the following commands to install it:
```shell
unzip protoc-xxx.zip -d protoc3
sudo cp protoc3/bin/* /usr/local/bin/
sudo cp protoc3/include/* /usr/local/include/
```

Also make sure to install `protoc-gen-go` by running:
```shell
go install google.golang.org/protobuf/cmd/protoc-gen-go
```

### Go-jsonschema
Nex uses go-jsonschema for validating the configuration files. You can install it by [following the installation instructions](https://github.com/omissis/go-jsonschema?tab=readme-ov-file#installing)

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

