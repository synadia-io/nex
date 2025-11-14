---
title: Core Concepts
description: How Nex schedules workloads across nexlets
sidebar_position: 4
---

# Core Concepts

Nex is a workload orchestration layer built on NATS. Nodes coordinate placement, nexlets execute workloads, and workloads communicate over scoped NATS credentials. This page explains the foundational vocabulary and flows that other guides build on.

## Architecture Overview

At runtime a Nex deployment consists of:

- **Nex nodes** – Control-plane processes that accept CLI/API requests, run placement auctions, mint workload credentials, and maintain workload state. A deployment can run one node for local development or multiple nodes connected to the same NATS cluster.
- **Nexlets (agents)** – Runtime adapters that implement the `Agent` interface from `github.com/synadia-labs/nex/sdk/go/agent`. Each nexlet advertises the workload type(s) and lifecycle(s) it supports (for example, `native` services) and receives start/stop instructions from a node.
- **Workloads** – Units of execution scheduled onto nexlets: services, jobs, or functions. Workloads inherit their behavior from the nexlet they target.
- **NATS cluster** – The messaging fabric that connects nodes, nexlets, workloads, and administrators. NATS subjects carry auctions, lifecycle commands, heartbeats, logs, and events.

```
┌──────────┐      auctions / control      ┌────────────┐
│  Client  │ ───────────────────────────► │   Nodes    │
└──────────┘                              └────┬───────┘
                                               │
                        start/stop, heartbeats │
                                               ▼
                                          ┌────────────┐
                                          │  Nexlets   │
                                          └────┬───────┘
                                               │
                                               ▼
                                          ┌────────────┐
                                          │ Workloads  │
                                          └────────────┘
```

All communication flows through NATS subjects so components can be distributed or collocated without changing the control plane.

## Key Entities

### Nex Node

Every node manages:

- **Agent supervision** – Accepts agent registrations, restores them on restart, and marks their health from heartbeats.
- **Auctions** – Responds to workload placement requests by matching nexlets against tags, type, and lifecycle.
- **Credential minting** – Issues scoped NATS credentials for workloads and remote nexlets using a configured signing strategy (signing key, user NKEY, or an insecure full-access fallback for development).
- **State** – Optionally persists workload assignments in a NATS Key-Value bucket so it can rebuild state after restarts.
- **Logging and events** – Ships workload logs and lifecycle events over `$NEX.FEED.*` subjects, and can emit structured events via pluggable emitters.

Nodes expose administrative operations (list, info, lameduck) over NATS and via the `nex` CLI.

### Nexlet (Agent)

Nexlets are runtime adapters that implement the `agent.Agent` interface:

```go
type Agent interface {
    Register() (*models.RegisterAgentRequest, error)
    Heartbeat() (*models.AgentHeartbeat, error)
    StartWorkload(workloadID string, req *models.AgentStartWorkloadRequest, existing bool) (*models.StartWorkloadResponse, error)
    StopWorkload(workloadID string, req *models.StopWorkloadRequest) error
    GetWorkload(workloadID, targetXKey string) (*models.StartWorkloadRequest, error)
    QueryWorkloads(namespace string, filter []string) (*models.AgentListWorkloadsResponse, error)
    SetLameduck(before time.Duration) error
    Ping() (*models.AgentSummary, error)
}
```

The SDK’s `Runner` handles the NATS plumbing so agent authors can focus on runtime logic (process management, container APIs, function invocations, etc.). Nexlets stream workload stdout/stderr/metrics via `Runner.GetLogger` and emit optional trigger responses for `function` lifecycles.

Nexlets can run:

- **Embedded** – Started by the node process (native nexlet by default).
- **Local** – Managed as child processes via the node’s `agents` configuration.
- **Remote** – Running on separate hosts; they register over NATS when `--allow-remote-agent-registration` is enabled.

### Workload

A workload definition includes:

- `name`, `description` – Human-readable identifiers.
- `type` – Matches the nexlet `register_type`.
- `lifecycle` – One of `service`, `job`, or `function`.
- `tags` – Placement hints that must match node/nexlet tags.
- `start_request` – Runtime-specific configuration validated against the nexlet’s JSON schema.
- `namespace` – Logical isolation boundary for credentials, logs, and events.

Workloads are typically defined in a `Nexfile` for repeatability. The CLI validates the `start_request` against the target agent’s schema before sending it to the node.

## Workload Lifecycles

- **Service** – Expected to run indefinitely. Nexlets usually restart services if the process exits unexpectedly.
- **Job** – Runs to completion; the nexlet reports terminal state back to the node.
- **Function** – Event-driven. The nexlet can register NATS triggers via `Runner.RegisterTrigger` and launch isolated executions per invocation.

Lifecycle support depends on the nexlet implementation; the agent advertises supported lifecycles during registration and the node enforces them during auctions.

## Placement and Execution Flow

1. **Auction request** – A client (CLI or SDK) submits workload requirements on `$NEX.SVC.<namespace>.control.AUCTION`, including workload type, lifecycle, and tags.
2. **Node bidding** – Each node examines its registered nexlets. If a nexlet matches the request, the node responds with a bid containing the agent ID, supported lifecycles, and the agent’s start request schema.
3. **Winner selection** – The client chooses one bid (the CLI currently picks a random eligible bid) and sends a deployment payload to the winning node at `$NEX.SVC.<namespace>.control.ADEPLOY.<bidder_id>`.
4. **Credential minting** – The node generates scoped workload credentials using its configured minter (signing key or user NKEY) and embeds them in the `AgentStartWorkloadRequest`.
5. **Agent invocation** – The node sends `StartWorkload` to the selected nexlet (`$NEX.SVC.<node_id>.agent.STARTWORKLOAD.<agent_id>.<workload_id>` via the SDK’s microservice endpoints). The nexlet starts the workload, attaches log streams, and acknowledges success or failure.
6. **State tracking** – If persistence is enabled, the node stores the workload definition so it can replay `StartWorkload` if the nexlet reconnects.

Stopping a workload follows the reverse path (`StopWorkload` message, optional trigger cleanup, credential revocation).

## Namespaces and Tags

- **Namespaces** partition workloads, logs, and events. Nodes default to the `system` namespace for administrative actions; user workloads typically live in `default` or a custom namespace.
- **Tags** provide scheduling metadata. Nodes store their own tags (e.g., `region=lab`, `arch=amd64`), and workloads can require specific tags (`--tags key=value`). Nexlets inherit the node’s tags and can augment them at registration.

Namespaces influence credential scoping: workloads can only publish and subscribe within their namespace unless a nexlet grants additional access.

## Security Model

- **Scoped credentials** – Every workload receives NATS credentials limited to the namespace and subjects it needs. Nexlets obtain their own scoped credentials during registration, and the node refreshes them on reconnection.
- **Curve (XKey) encryption** – Nodes and nexlets exchange public curve keys so sensitive payloads (environment variables, secrets) can be encrypted per workload.
- **Credential minting strategies** – Configure either a signing key + root account or a user NKEY/seed. The `FullAccessMinter` fallback exists only for local experimentation.

## Observability

- **Heartbeats** – Nexlets send `AgentHeartbeat` messages every 10 seconds. Nodes aggregate them into health summaries accessible via `nex node info`.
- **Logs** – Workload stdout/stderr and optional metrics stream on `$NEX.FEED.<namespace>.logs.<workload_id>.<out>` and `$NEX.FEED.<namespace>.metrics.<workload_id>`. Subscribe with the NATS CLI or integrate with your own log collectors.
- **Events** – Node, agent, and workload events publish to `$NEX.FEED.<namespace>.event.*`. Nodes can emit events via the NATS emitter or suppress them with the no-op emitter.
- **Metadata** – `nex workload list --show-metadata` surfaces nexlet-specific state (container IDs, process IDs, trigger subjects, etc.) retrieved from `GetWorkload`.

## Programmatic Access with the Go Client

The `nex` CLI is a thin wrapper around the Go client package located at `github.com/synadia-labs/nex/client`. Use this library when you want to automate node or workload management without invoking the CLI.

Key features:

- Establishes a namespaced control plane client over an existing `*nats.Conn`.
- Wraps placement auctions (`Auction`), workload lifecycle methods (`StartWorkload`, `StopWorkload`, `CloneWorkload`, `ListWorkloads`), and node operations (`ListNodes`, `GetNodeInfo`, `SetLameduck`).
- Provides configurable timeouts and request cadence via functional options (for example `WithDefaultTimeout`, `WithStartWorkloadTimeout`).
- Supports concurrent request/response fan-out using `natsext.RequestMany`, matching the CLI’s behavior.

Basic usage:

```go
package main

import (
    "context"
    "log"

    "github.com/nats-io/nats.go"
    nexclient "github.com/synadia-labs/nex/client"
    "github.com/synadia-labs/nex/models"
)

func main() {
    nc, err := nats.Connect("nats://localhost:4222")
    if err != nil {
        log.Fatal(err)
    }
    defer nc.Drain()

    ctx := context.Background()
    c, err := nexclient.NewClient(ctx, nc, "default")
    if err != nil {
        log.Fatal(err)
    }

    bids, err := c.Auction("native", map[string]string{"region": "lab"})
    if err != nil {
        log.Fatal(err)
    }
    if len(bids) == 0 {
        log.Fatal("no agents available")
    }

    resp, err := c.StartWorkload(
        bids[0].BidderId,
        "hello",
        "demo service",
        `{"uri":"file:///usr/local/bin/hello"}`,
        "native",
        models.WorkloadLifecycleService,
        bids[0].Tags,
    )
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("started workload %s (%s)", resp.Name, resp.Id)
}
```

Because the client uses the same subjects as the CLI, it interoperates seamlessly with existing nodes and nexlets. Adopt it from services, operators, or CI/CD systems when you need direct control over the platform.

## State Persistence

Nodes can operate in-memory or attach a persistence backend:

- **NATS Key-Value (`--state kv`)** – Stores workload definitions keyed by `<register_type>_<workload_id>`. On restart, the node replays each entry by calling `StartWorkload(..., existing=true)` on the registered nexlet.
- **No state (`""`)** – Volatile mode suitable for quick experiments; workloads do not survive node restarts.

Custom persistence layers can implement the `models.NexNodeState` interface if you need an alternative store.

## Node and Nexlet Lifecycle

1. **Node startup**
   - Generates or loads node seeds and curve keys.
   - Connects to NATS (or starts an embedded server).
   - Starts embedded/local nexlets and waits for remote registrations.
   - Restores persisted workloads if state is enabled.
2. **Agent registration**
   - Agent submits a `RegisterAgentRequest` describing capabilities.
   - Node validates the request, assigns an agent ID, and returns connection credentials plus any existing workloads it should restore.
3. **Lameduck mode**
   - Node stops participating in auctions.
   - Existing workloads continue until the configured delay elapses.
   - After the delay, the node stops workloads (if instructed) and shuts down cleanly.

## Subject Naming Cheatsheet

| Purpose | Subject prefix |
| --- | --- |
| Control plane (auctions, deploy, stop, info) | `$NEX.SVC.<namespace>.control.*` |
| Agent microservice endpoints | `$NEX.SVC.<node_id>.agent.*` |
| Workload logs | `$NEX.FEED.<namespace>.logs.<workload_id>.*` |
| Workload metrics | `$NEX.FEED.<namespace>.metrics.<workload_id>` |
| Events | `$NEX.FEED.<namespace>.event.*` |
| Heartbeats | `$NEX.SVC.<node_id>.agent.HEARTBEAT.<agent_id>` |

You rarely need to publish directly to these subjects; the CLI and SDK abstract the messaging patterns. The subject layout is useful when integrating observability pipelines or writing custom automation.

## Putting It All Together

- Start a node with credential minting configured and at least one nexlet registered.
- Package workloads in Nexfiles so the CLI can validate and submit them.
- Use namespaces and tags to separate teams, regions, or runtime constraints.
- Subscribe to `$NEX.FEED.*` subjects to monitor behavior in real time.
- Leverage the Go SDK or NATS subjects directly if you need programmatic control beyond the CLI.

With these concepts in mind, dive into **Running Nex Nodes**, **Running Workloads**, and **Creating Custom Nexlets** for task-oriented instructions.
