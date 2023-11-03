# NATS Execution Engine
Turn your NATS infrastructure into a distributed workload deployment and execution engine.

* [nex-agent](./nex-agent) - Agent that runs inside a Firecracker VM, responsible for running untrusted workloads. Not something end users need to interact with.
* [nex-node](./nex-node) - Service running on a NEX node. Exposes a control API, starts/tops firecracker processes, communicates within the agent inside each process.
* [control-api](./control-api/) - The API for communicating with and remotely controlling NEX nodes
* [nex-cli](./nex-cli) - CLI for communicating with NEX nodes
* [fc-image](./fc-image/) - Tools for building the rootfs (ext4) file system for use in firecracker VMs
* [agent-api](./agent-api/) - Protobuf API specification for protocol used between `nex-node` and `nex-agent` across the firecracker boundary. Also contains a client for communicating with an agent. This is an internal API unlikely to be of interest to users.

## Deploying and Running Workloads
In order to create a deployable workload, you'll need to create a _statically linked_ executable compiled for 64-bit Linux. Creating a truly statically linked executable file can be tricky. The following shows how to do it in Go (run this in a directory with a `main.go` file):

```go
#!/bin/bash

set -xe

export GIN_MODE=release
go build -tags netgo -ldflags '-extldflags "-static"'
```

Building a statically linked 🦀 Rust binary involves using cargo to build against the `x86_64-unknown-linux-musl` target (or `aarch64` if you've deployed `nex-node` on ARM/Apple).

For other languages, check the language documentation or community.

Once your static executable is ready to run, you can use the [CLI](./nex-cli/) or the [Control API](./control-api/) to launch it.