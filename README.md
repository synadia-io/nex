# Nex

Nex is the NATS Execution Engine—a lightweight way to run distributed workloads over NATS without standing up heavy orchestration. Nodes auction work to nexlets (runtime agents) so you can run native binaries today and extend the platform with custom runtimes when you need them.

## Overview

- Simple, imperative deployment model backed by NATS messaging.
- Works with native executables out of the box; or bring your own runtime by writing a pluggable Nexlet.
- Every workload receives scoped NATS credentials, making pub/sub, request/response, and JetStream access immediate.

## Installation

- **Pre-built binaries:** Download the latest release for your platform from [github.com/synadia-io/nex/releases](https://github.com/synadia-io/nex/releases) and place `nex` on your `PATH`.
- **Go install:** `go install github.com/synadia-io/nex/cmd/nex@latest`
- **From source:**

  ```bash
  git clone --depth 1 https://github.com/synadia-io/nex.git
  cd nex
  go build -o nex ./cmd/nex
  ```

## Quickstart

1. **Start NATS** (or point to an existing server):

   ```bash
   nats-server -a 127.0.0.1 -p 4222
   ```

2. **Start a Nex node** (started with the native nexlet by default):

   ```bash
   nex -s nats://127.0.0.1:4222 node up
   nex -s nats://127.0.0.1:4222 node list  # verify the node is registered
   ```

3. **Deploy a workload.** Create a `Nexfile`:

   Note: Nexfiles are written in YAML.

   ```yaml
   name: "system-reporter"
   description: "Reports system stats every 30 seconds"
   type: native
   lifecycle: service
   start_request:
     uri: "file:///bin/sh"
     argv:
       - "-c"
       - "while true; do echo \"[$(date '+%Y-%m-%d %H:%M:%S')] Uptime: $(uptime)\"; sleep 30; done"
   ```

   Deploy and observe:

   ```bash
   nex -s nats://127.0.0.1:4222 workload start -f Nexfile
   nex -s nats://127.0.0.1:4222 workload ls
   ```

For deeper guides, concepts, and nexlet development, visit our docs site at [docs.synadia.com/oss/nex](https://docs.synadia.com/oss/nex).
