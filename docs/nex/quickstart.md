---
title: Quick Start
description: Get started with Nex in 5 minutes
sidebar_position: 2
---

# Quick Start

Get Nex up and running in minutes. This guide will walk you through installation, starting your first node, and deploying your first workload.

## Prerequisites

Before you begin, make sure you have:

- Go 1.24+ (if you are building Nex from source)
- `nats-server` ([installation instructions](https://docs.nats.io/running-a-nats-service/introduction/installation) from [docs.nats.io](https://docs.nats.io/))

## Nex CLI Installation

Here's a couple of options for installing the Nex CLI, which can be used to run Nex nodes and manage/provision workloads:

### Pre-built Binaries

Visit [github.com/synadia-io/nex/releases](https://github.com/synadia-io/nex/releases) to download the latest release for your OS and architecture.

### Using Go Install

```bash
go install github.com/synadia-io/nex/cmd/nex@latest
```

### From Source

```bash
# Clone latest (main)
git clone --depth 1 https://github.com/synadia-io/nex.git nex.git
# OR clone the latest release
git clone --depth 1 --branch <latest-release-tag> https://github.com/synadia-io/nex.git nex.git

# Build the CLI
go build -o nex ./cmd/nex
```

## Start Your First Node

First, start a NATS server:

```bash
nats-server -a 127.0.0.1 -p 4222
```

In a new terminal, start a Nex node that connects to your NATS server:

```bash
nex -s nats://127.0.0.1:4222 node up
```

Verify the node is running by listing all connected nodes:

```bash
nex -s nats://127.0.0.1:4222 node list
```

Your Nex node is now running with the native nexlet included by default. This nexlet can execute native binaries on your host.

## Deploy Your First Workload

### Deploy a Service

Services are long-running processes that restart on failure.

Create a Nexfile (workload configuration file) named `nexfile.yaml`. This example runs a native executable binary (`/bin/sleep`) using the embedded native nexlet:

```yaml
name: "sleepy-guy"
description: "zzzzzzz"
type: native  # Deploys only to nexlets of type 'native'
lifecycle: service
start_request:
  uri: "file:///bin/sleep"  # Native binary to execute
  argv:
    - "1d"
```

Deploy the workload:

```bash
nex -s nats://127.0.0.1:4222 workload start -f nexfile.yaml
```

The workload will be auctioned to available nodes and deployed to the winning bidder.

List running workloads to verify deployment:

```bash
nex -s nats://127.0.0.1:4222 workload ls
```

### Deploy a Service and View Workload Logs

Create a Nexfile named `system-reporter.yaml`:

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

Deploy the workload:

```bash
nex -s nats://127.0.0.1:4222 workload start -f system-reporter.yaml
```

View the workload logs by viewing messages emitted to the logs NATS subject:

```bash
nats -s nats://127.0.0.1:4222 sub '$NEX.FEED.*.logs.>'
```

You'll see system stats output every 30 seconds, confirming your workload is running.


## Basic Operations

### List Workloads

```bash
nex -s nats://127.0.0.1:4222 workload ls
```

### Stop a Workload

```bash
nex -s nats://127.0.0.1:4222 workload stop <workload-id>
```

## Troubleshooting

### Common Issues

**Node won't start:**
- Check if the `nats-server` is running and you have supplied Nex the correct URL/authentication

**Workload deployment fails:**
- Ensure the node has a nexlet that supports your runtime
- Check node logs for detailed error messages

<!--## What's Next?-->

<!--Now that you have Nex running:-->

<!--TODO-->

## Getting Help

- Join `#nex` channel on [NATS Slack](https://slack.nats.io)
- Report issues on [GitHub](https://github.com/synadia-io/nex/issues)
