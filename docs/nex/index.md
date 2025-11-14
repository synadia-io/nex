---
title: Nex
description: NATS Execution Engine - Distributed Workload Execution Platform
pagination_next: oss/nex/quickstart
sidebar_position: 1
---

# Nex - NATS Execution Engine

Nex is a distributed workload execution system built on NATS messaging. It provides a simple, imperative platform for running workloads across distributed nodes.

## What is Nex?

Nex brings compute to your NATS infrastructure without complex orchestration layers. At its core, Nex provides:

- **Simple, imperative workload deployment** - No complex management layer required. Deploy workloads directly and build your own scheduling, resilience, and self-healing logic on top if needed.

- **Run anything, anywhere** - Support for containers, native processes, WASM, VMs, and custom runtimes through pluggable nexlets.

- **NATS-native workloads** - Every workload automatically receives scoped NATS credentials and a connection, enabling you to build applications with all the benefits of NATS: pub-sub, request-reply, queue groups, JetStream, and more.

### Nex Node
The central orchestrator that manages nexlet lifecycle and handles workload deployment. It can run with an embedded or external NATS server.

### Nexlets
Execution agents that handle the actual workload lifecycle (start, stop, query, heartbeat). Nexlets can be:
- **Embedded** - Built into the node binary (like the included native nexlet)
- **Local** - Separate process on the same machine
- **Remote** - Running on different machines

Nex includes a native nexlet for executing OS processes. You implement custom nexlets for other runtimes (containers, VMs, WASM) using the Go SDK.

## Workload Types

Nex supports three fundamental workload types:

| Type | Description | Use Cases |
|------|-------------|-----------|
| **Job** | Run-to-completion tasks | Batch processing, data pipelines, one-time operations |
| **Service** | Long-running processes with auto-restart | Web servers, background workers, daemons |
| **Function** | Event-driven, triggered via NATS subjects | Webhooks, event handlers, request processors |

## Why Nex?

### Simplicity First
Unlike traditional orchestrators with complex control planes, Nex keeps things simple:
- **Minimal architecture** - Just Nex nodes and nexlets (runtime agents), with all internal communication over NATS
- **No additional infrastructure** - No API servers, etcd clusters, or external dependencies beyond NATS
- **Extensible foundation** - Build auto-scaling, custom scheduling, or self-healing on top using Nex's emitted events

### NATS-Native Workloads
Every workload deployed on Nex automatically receives:
- **Scoped NATS credentials** - Pre-configured authentication and authorization
- **NATS connection** - Ready-to-use connection for pub-sub, request-reply, and queue groups
- **Service mesh capabilities** - Service discovery, load balancing, and RPC through NATS
- **Integrated observability** - Logs and events published to NATS subjects

### Flexible Runtime Support
- **Pluggable nexlets** - Support for any runtime through the nexlet interface
- **Multiple workload types** - Jobs (run-to-completion), Services (long-running), Functions (event-triggered)
- **Simple SDK** - Create custom nexlets using the Go SDK

## Next Steps

- 🚀 Follow the [Quick Start Tutorial](./quickstart.md) to deploy your first workload
- 💬 Join the `#nex` channel in [NATS Slack](https://slack.nats.io) community for support

## Resources

- [GitHub Repository](https://github.com/synadia-io/nex)
- [NATS Documentation](https://docs.nats.io)
