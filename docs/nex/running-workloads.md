---
title: Running Workloads
description: Deploying, inspecting, and managing workloads with the Nex CLI
sidebar_position: 3
---

# Running Workloads

A Nex workload is a unit of execution scheduled onto a nexlet. This guide shows how to prepare a workload definition, submit it to a node, and operate it throughout its lifecycle. It assumes you already have a running node (see **Running Nex Nodes**) and at least one nexlet capable of the workload type you plan to run (for example, the bundled native nexlet).

## Prerequisites

- **CLI**: Build or install the `nex` CLI (`task nex` or `go build ./cmd/nex`) and add it to your `PATH`.
- **NATS context**: Configure and validate a NATS CLI context that can reach the cluster used by your node ([docs](https://github.com/nats-io/nats.docs/blob/master/using-nats/nats-tools/nats_cli/README.md#nats-contexts)):
  ```bash
  nats context add nex-dev --server nats://demo.nats.io:4222 --creds /path/to/account.creds
  nats context use nex-dev
  nats context info nex-dev
  nats ping --context nex-dev
  ```
  The `info` and `ping` commands confirm authentication and connectivity.
- **Node access**: Start a node and verify an agent is registered:
  ```bash
  nex --namespace system node up --nats.context nex-dev
  # In another terminal:
  nex --namespace system node list
  ```
  Replace `node up` with your preferred start command (config file, remote nexlets, etc.).
- **Config file (optional)**: If you keep reusable settings in `config.json`, include `--config path/to/config.json` on every CLI call or rely on the default locations (`/etc/nex/config.json`, `~/.config/nex/config.json`, `./config.json`).

Throughout this guide, replace `nex-dev` and namespaces with values that match your environment.

## Understand Workload Types and Lifecycles

- **Type (`--type`)**: Corresponds to the nexlet `register_type`. Only agents advertising that type participate in the placement auction. Native workloads use `native`.
- **Lifecycle (`--lifecycle`)**:
  - `service`: long-running processes expected to stay up until stopped.
  - `job`: finite tasks that complete and exit.
  - `function`: event-driven workloads that may register triggers; runtimes supply additional invocation metadata.

The node rejects requests when no agent matches both the type and lifecycle.

## Define a Workload with Nexfile

A `Nexfile` (JSON or YAML) captures workload metadata, auction hints, and the nexlet-specific `start_request`. The CLI will auto-discover a file named `Nexfile` in the working directory; you can override the path with `--nexfile`.

```yaml title="Example Nexfile for the native nexlet"
name: hello-exec
description: simple native service
type: native
lifecycle: service
tags:
  region: lab
start_request:
  uri: "file:///usr/local/bin/hello-service"
  argv: ["--listen=:8080"]
  environment:
    LOG_LEVEL: info
  expose_ports: [8080]
```

The `start_request` section must match the schema the nexlet published during registration. For the native nexlet (`agents/native/start_request.json`), supply:

- `uri` (required): `file:///` path to the executable on the agent host.
- Optional fields: `argv`, `environment`, `workdir`, `stdin`, `expose_ports`, `artifacts`, etc.

**Tip:** Keep Nexfiles alongside application code so you can version-control workload definitions.

## Start a Workload

1. Select the namespace you’ll deploy into (defaults to `system`; change it if you segregate workloads):
   ```bash
   nex --namespace default workload list
   ```
   This verifies connectivity and ensures the namespace exists.
2. Launch the workload referencing your Nexfile:
   ```bash
   nex --config ./config.json --namespace default workload start --nexfile Nexfile
   ```
   - The CLI validates the `start_request` against the agent’s schema before submission.
   - The node auctions the request across eligible agents. If multiple agents bid, one is chosen at random.
   - Successful placement prints the workload ID and name: `Workload hello-exec [ww2TFc...] successfully started`.
3. Override any Nexfile value with flags as needed, for example `--tags region=prod --tags arch=amd64` to target specific nodes or `--name api` to rename the workload.

When no Nexfile is present, provide the required fields inline:

```bash
nex --namespace default workload start \
  --type native \
  --lifecycle job \
  --name onetime-task \
  --start-request '{"uri":"file:///usr/local/bin/job","argv":["--once"]}'
```

Inline JSON must already satisfy the agent’s schema; the CLI performs the same validation step and returns any schema errors before contacting the node.

## Inspect Running Workloads

Use `nex workload list` to view workload state aggregated across agents:

```bash
nex --namespace default workload list
```

Add `--show-metadata` to include nexlet-provided metadata (for example, native process IDs, container names, or custom runtime state). Use `--type <register_type>` to filter by workload type. The CLI fetches data directly from every agent; expect a short delay while the node polls each nexlet.

For JSON output (scripting or automation), include `--json`.

## Stop and Clone Workloads

- **Stop**: `nex --namespace default workload stop <workload_id>` gracefully stops the workload using the nexlet’s implementation (`StopWorkload`). Jobs that already exited appear as stopped when listed.
- **Clone**: `nex --namespace default workload clone <workload_id> --tags region=canary` re-auctions the same definition onto fresh capacity. Append `--stop` to stop the original instance after the clone succeeds.
- **Redeploy adjustments**: Modify your Nexfile (new command, updated environment, resource tweaks) and re-run `workload start` with the same `--name` if you want to treat it as a replacement. The new workload receives a new ID; stop the old one when you confirm success.

## Observe Logs and Events

Workload logs, metrics, and events stream through NATS subjects prefixed with `$NEX.FEED.<namespace>`. Use the NATS CLI or your preferred tooling to subscribe:

```bash
# Workload stdout/stderr
nats --context nex-dev sub "$NEX.FEED.default.logs.>"

# Workload lifecycle events (started/stopped/triggered)
nats --context nex-dev sub "$NEX.FEED.default.event.>"
```

To filter a specific workload, replace `>` with the workload ID. Combine these subscriptions with `nex workload list` to correlate state changes during rollouts or incident response.

## Handling Common Scenarios

- **Auction returns “no agents available”**: Ensure at least one nexlet is registered with the requested `type` and includes the lifecycle in its `supported_lifecycles`. Adjust node or nexlet tags to match the workload’s `tags`.
- **Schema validation errors**: Inspect the error message; it references the field that failed validation. Compare against the nexlet’s published schema (`synadia-labs/nex/agents/<type>/start_request.json` in the monorepo).
- **Long-running job appears stuck**: Check `nex workload list --show-metadata` for nexlet-specific hints. Some runtimes report last-heartbeat times or internal states in metadata.
- **Logging too noisy**: Disable workload logs in node output with `nex node up --show-workload-logs=false` (default). Subscribe directly to the NATS subjects to view logs on demand.
- **Namespace confusion**: Nodes default to the `system` namespace for administrative operations, but you can deploy workloads to any namespace. Ensure your CLI `--namespace` matches the workload definitions and log subscriptions.

## Next Steps

- Review **Creating Custom Nexlets** to understand how start request schemas are defined and how credentials reach workloads.
- Explore the Nex client API (`github.com/synadia-labs/nex/client`) if you plan to automate workload submissions or build higher-level tooling.
- When ready to package workloads, watch the repository for the upcoming `workload bundle` functionality (currently commented out in the CLI).
