---
title: Creating Custom Nexlets
description: Build runtime-specific execution agents for Nex
---

# Creating Custom Nexlets

Nexlets are runtime-specific agents that run workloads on behalf of Nex nodes. Each nexlet advertises what it can execute, receives workload instructions over NATS, and manages lifecycle, telemetry, and recovery for those workloads. This guide walks through designing and implementing a nexlet, using a concise “noop” runtime example that simply records lifecycle calls. Swap the noop pieces for your own runtime implementation.

## How Nexlets Fit Into Nex

- A Nex node auctions new workloads to agents whose `register_type` matches `workload_type` in the workload request.
- When a nexlet process starts it connects to the node through the Go SDK runner, calls `Register()`, and exposes NATS microservice endpoints for lifecycle operations.
- The node drives workload execution by sending `StartWorkload`, `StopWorkload`, `GetWorkload`, and `QueryWorkloads` requests to the nexlet's runner.
- Nexlets stream logs, metrics, and events back through the runner, enabling observability and trigger mechanisms inside Nex.

Understanding this flow up front keeps your agent aligned with the platform contracts enforced by `github.com/synadia-labs/nex/sdk/go/agent`.

## Prerequisites

- Go 1.21 or later (Go 1.24 for newest agents).
- Working knowledge of your runtime's lifecycle APIs (start, stop, inspect, log, metrics, cleanup).
- Schema tooling when you plan to generate typed config (e.g. `go install github.com/atombender/go-jsonschema`).

## Design Checklist

1. Runtime capabilities – list the workload lifecycles (job, service, function) you can support and any concurrency limits.
2. Register type – choose a unique `register_type` string. Workloads targeting your agent must set `workload_type` to this value.
3. Workload contract – design the `start_request` schema that captures how developers describe workloads your runtime will execute (image, command, CPU, etc.).
4. State management – decide how you will track running workloads so `GetWorkload` and `QueryWorkloads` remain accurate and recovery can rehydrate state.
5. Observability – map runtime logs, metrics, and events into Nex log streams and any additional telemetry surfaces you choose to expose.
6. Credential handoff – define how workloads receive their scoped NATS credentials (env vars, mounted files, init scripts).
7. Security – identify secrets your runtime needs and plan to pull them with `runner.GetNamespaceSecret`.
8. Packaging – determine how you will ship and configure the binary (build scripts, systemd unit, container image, etc.).

Reference the native nexlet (`synadia-labs/nex/agents/native`) as you work through the checklist.

## Bootstrap the Agent

### 1. Initial project skeleton

Create a dedicated module for your nexlet and pull in the shared runner:

```bash
go mod init github.com/acme/nexlet
go get github.com/synadia-labs/nex/sdk/go/agent
```

Organize your code similar to the native nexlet: keep `agent/` for the interface implementation, place runtime adapters under `internal/` (or `runtime/`), maintain schemas in `models/`, and add a `runner/` wrapper if you need custom setup.

### 2. Wire up the runner

The runner manages connectivity to the Nex node, exposes NATS endpoints, and handles heartbeats. A trimmed entry point looks like this:

```go
package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"
    "syscall"

    nexagent "github.com/synadia-labs/nex/sdk/go/agent"
    "github.com/acme/nexlet/agent"
    "github.com/acme/nexlet/internal/noop"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    runtime := noop.NewRuntime(logger)

    customAgent := agent.New(ctx, runtime, logger)

    runner, err := nexagent.NewRunner(ctx, os.Getenv("NEXUS_ID"), os.Getenv("NODE_ID"), customAgent,
        nexagent.WithLogger(logger),
    )
    if err != nil {
        logger.Error("failed to create runner", "err", err)
        os.Exit(1)
    }

    if err := customAgent.Init(runner); err != nil {
        logger.Error("failed to initialise agent", "err", err)
        os.Exit(1)
    }

    creds := loadConnectionData()
    emitter := loadEventEmitter()

    if err := runner.Run(creds.AgentID, creds.ConnectionData, emitter); err != nil {
        logger.Error("runner aborted", "err", err)
        os.Exit(1)
    }
}
```

The `internal/noop` package in this example exposes a minimal runtime implementation that records lifecycle events and simulates work based on the `start_request` payload. Swap it for your actual runtime client once you are ready.

In a remote deployment you can obtain `AgentID` and connection data via `nexagent.RemoteAgentInit`. Tie `loadConnectionData()` and `loadEventEmitter()` to your provisioning story (static files, environment, or remote registration).

### 3. Implement the agent

Your agent struct implements `agent.Agent` from the SDK. Embed dependencies you need to service requests (runtime client, state store, logger):

```go
type Agent struct {
    ctx     context.Context
    logger  *slog.Logger
    runtime *noop.Runtime
    runner  *nexagent.Runner
    state   *StateStore
    kp      nkeys.KeyPair
    version string
}
```

Expose constructor helpers (`agent.New`) that prepare the key pair, instantiate state, and return the struct ready for `Init(runner)`.

The noop runtime can be as small as:

```go
package noop

import (
    "fmt"
    "io"
    "log/slog"
    "time"
)

type Runtime struct {
    logger *slog.Logger
}

func NewRuntime(logger *slog.Logger) *Runtime {
    return &Runtime{logger: logger}
}

func (r *Runtime) Start(workloadID string, cfg NoopStartRequest, stdout, stderr io.Writer) error {
    fmt.Fprintf(stdout, "started noop workload %s: %s\n", workloadID, cfg.Label)
    if cfg.Message != "" {
        fmt.Fprintln(stdout, cfg.Message)
    }
    if cfg.SleepMs > 0 {
        time.Sleep(time.Duration(cfg.SleepMs) * time.Millisecond)
    }
    return nil
}

func (r *Runtime) Stop(workloadID string) error {
    r.logger.Info("stop noop workload", "workload_id", workloadID)
    return nil
}
```

This keeps the example focused on the Nex integration points while leaving room for your actual runtime logic.

## Workload schema and configuration

The Nex node enforces the JSON schema you return from `Register()`. Store the schema in `models/start_request.json` and keep it authoritative. Remember: this schema expresses workload inputs (what to run, with which resources), not how to configure the nexlet process itself.

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "NoopStartRequest",
  "type": "object",
  "properties": {
    "label": {
      "type": "string",
      "description": "Friendly name to include in logs and events"
    },
    "message": {
      "type": "string",
      "description": "Optional message the noop runtime can echo"
    },
    "sleep_ms": {
      "type": "integer",
      "minimum": 0,
      "description": "Delay before reporting completion"
    }
  },
  "required": ["label"]
}
```

Generate Go types from the schema so `StartWorkload` can unmarshal safely:

```bash
go generate ./...
```

Each shipping nexlet uses `go:generate` with `github.com/atombender/go-jsonschema` to keep the generated file (`models/gen_start_request.go`) in sync.

## Lifecycle method guidance

### Register

Return metadata that describes your agent:

- `Name` – human readable identifier (mirror the native agent’s naming pattern).
- `RegisterType` – matches user supplied workload `--runtime`.
- `SupportedLifecycles` – slice of `models.WorkloadLifecycle` constants that you actually implement.
- `StartRequestSchema` – schema string loaded via `//go:embed`.
- `PublicXkey` – obtained from the agent’s curve key pair; the node encrypts secrets with it.
- `MaxWorkloads` – set to 0 if unlimited, or the number of concurrent workloads you can host.

See `synadia-labs/nex/agents/native/native.go` for a complete reference implementation.

### Heartbeat and Ping

`Heartbeat()` should summarise resource usage, counts, or custom stats. Serialize lightweight data to `AgentHeartbeat.Data`. The runner pushes heartbeats every ten seconds. `Ping()` returns the latest `AgentSummary` and is used for node health probes.

### StartWorkload

`StartWorkload(workloadID string, req *models.AgentStartWorkloadRequest, existing bool)` is the core of your agent:

1. Decode `req.Request.RunRequest` into your generated struct.
   ```go
   var cfg NoopStartRequest
   if err := json.Unmarshal([]byte(req.Request.RunRequest), &cfg); err != nil {
       return nil, fmt.Errorf("invalid noop request: %w", err)
   }
   ```
2. Pull any secrets referenced via `runner.GetNamespaceSecret`.
3. Prepare workload-scoped NATS credentials. `req.WorkloadCreds` contains connection data you must surface to the workload (environment variables, config files, or runtime-specific secret stores).
4. Optionally resolve artifacts (images, kernels) before scheduling to surface errors early.
5. Provision runtime resources (containers, microVMs, subprocesses, or—in the noop example—an in-memory record of the workload). If your runtime exposes stdout/stderr, pipe the output through log writers from `runner.GetLogger`:
   ```go
   stdout := a.runner.GetLogger(workloadID, req.Request.Namespace, models.LogOutStdout)
   stderr := a.runner.GetLogger(workloadID, req.Request.Namespace, models.LogOutStderr)

   if err := a.runtime.Start(workloadID, cfg, stdout, stderr); err != nil {
       return nil, fmt.Errorf("runtime start failed: %w", err)
   }
   ```
6. Start metrics collection by writing JSON payloads to `models.LogOutMetrics` when available.
7. Persist workload metadata in your state store so `GetWorkload` and `QueryWorkloads` can respond quickly.
8. Emit `models.WorkloadStartedEvent` through `runner.EmitEvent`, populating namespace and workload type.
9. If `existing` is true you are rehydrating after a node restart; skip provisioning work that already exists and just rebuild state.

Mirror the patterns in `synadia-labs/nex/agents/native/state.go`.

### StopWorkload

Implement idempotent teardown:

- Abort ongoing operations and detach log streams.
- Remove runtime resources (stop container, delete VM).
- Call `runner.UnregisterTrigger` if you registered any function triggers.
- Emit `models.WorkloadStoppedEvent`.
- Update local state so queries reflect the removal.

### GetWorkload and QueryWorkloads

Return the canonical `models.StartWorkloadRequest` representation. Keep fast lookup structures keyed by workload ID and filtered by namespace. Study `synadia-labs/nex/agents/native/state.go` for production patterns and `_test/nexlet_inmem/inmemagent.go` for a minimal in-memory approach.

### SetLameduck

Prepare for graceful shutdown by refusing new work and draining existing workloads. Many agents store a flag and reuse `Heartbeat` to report `lameduck` state.

## Observability and integrations

**Logs and metrics** – Use `runner.GetLogger` to stream stdout, stderr, and metrics to Nex. Layer in runtime-specific telemetry (HTTP endpoints, host exporters, etc.) as needed for your environment.

**Events** – Call `runner.EmitEvent` with `models.WorkloadStartedEvent`, `models.WorkloadStoppedEvent`, or custom structs. The node injects an emitter implementation when `runner.Run` starts.

**Function triggers** – Workloads of type `function` can register trigger subjects. Call `runner.RegisterTrigger` with the subject and handler; remember to unregister on stop. The in-memory nexlet (`synadia-labs/nex/_test/nexlet_inmem`) demonstrates this flow.

**Ingress** – Implement `agent.AgentIngessWorkloads` when you can determine exposed ports. Combine with `nexagent.WithIngressSettings` so the runner notifies the ingress manager.

**Event listeners** – If your runtime should react to node events implement `agent.AgentEventListener` and handle messages under `EventListener`.

**Secrets** – Accept values in `run_request` that start with `secret://` and fetch them with `runner.GetNamespaceSecret(namespace, secretID)`.

## State management and recovery

The node may restart or reconnect the agent with `RegisterAgentResponse.ExistingState`. When `existing` is true in `StartWorkload`, assume the workload is already running and rebuild local state without recreating resources. The native agent’s state management (`agents/native/state.go`) illustrates how to restore workloads after reconnects.

Persist transient data you need for heartbeat summaries or reconciliation, but avoid long-term secrets or user payloads on disk unless your runtime requires it.

## Testing strategy

- Unit test lifecycle methods using mocks or fakes of your runtime. The native agent’s tests (`synadia-labs/nex/agents/native/native_test.go`) and `_test/nexlet_inmem` provide good starting points.
- Add table-driven tests for schema validation, memory limits, and error propagation.
- Exercise rehydration by invoking `StartWorkload` with `existing=true`.
- Run `go test ./...` as part of your CI and mirror whatever automation your team relies on (Makefile, scripts, CI pipelines).
- Keep smoke scenarios under `_examples/` if you publish reference workloads.

## Packaging and deployment

Decide how operators will run the agent:

- Provide configuration flags or environment variables for node ID, nexus ID, runtime sockets, and optional ingress IPs. Keep option parsing in a dedicated package for reuse and testability.
- Provide build scripts or Makefile targets for cross compilation as needed (linux/amd64, darwin/arm64, etc.).
- Document runtime-specific system requirements (host sockets, kernel capabilities, filesystem expectations) in your README.
- Bundle service units or container manifests as needed.

## Reference implementations

- `synadia-labs/nex/agents/native` – minimal native binaries process runner which deploys jobs and services.

