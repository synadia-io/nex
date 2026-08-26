---
title: Workload Control API
description: Wire-level reference for the node control API — subjects, request/response semantics, and what each verb is expected to do
sidebar_position: 6
---

# Workload Control API

This is the wire-level reference for the node control API — what the CLI and SDK
speak underneath. Read **Running Workloads** first for the operator view; read
this when you are building a client, a control plane, or debugging what a verb
actually did.

Every control request is a NATS request on a subject of the form:

```
$NEX.SVC.<namespace>.control.<VERB>[.<id>]
```

Request and response bodies are JSON; their authoritative shapes are the JSON
Schemas in [`models/schema/`](https://github.com/synadia-io/nex/tree/main/models/schema)
(e.g. `update-workload-request.json`). Errors are returned as NATS micro
service errors (`Nats-Service-Error` / `Nats-Service-Error-Code` headers).

## Namespaces

The `<namespace>` token scopes every verb:

- A workload belongs to exactly one namespace, fixed at deploy time.
- A caller addressing a workload in another namespace is treated as addressing
  an unknown workload (see *silent drop* below) — namespaces are an isolation
  boundary, not a routing hint.
- The `system` namespace is administrative: system-namespace callers may
  address workloads in any namespace, and listing under `system` aggregates
  all namespaces.

## Reply conventions

Three patterns cover every verb:

- **Scatter-gather** (`AUCTION`, `PING`, `WPING`): every eligible node
  replies; the client collects replies until its stall window closes.
- **Single-owner** (`ADEPLOY`, `UNDEPLOY`, `CLONE`, `UPDATE`, `RESTART`):
  only the node that owns the bid or the workload replies. All other nodes
  stay silent.
- **Silent drop**: a node that receives a single-owner verb for a workload it
  does not hold — including one that exists but belongs to another
  namespace — does **not** reply at all. The client reads the resulting
  timeout as "workload not found". This is deliberate: replying "not mine"
  or "wrong namespace" would let a caller enumerate which IDs exist
  (an existence oracle). Clients must size their request deadline above the
  owner's worst-case handling so a slow success is never misread as
  not-found; the reference client uses a per-request budget of 90s against a
  node worst case of ~78s.

## Verbs

| Verb | Subject | Request → Response schema |
|---|---|---|
| Auction | `…control.AUCTION` | `auction-request` → `auction-response` |
| Deploy | `…control.ADEPLOY.<bidder_id>` | `start-workload-request` → `start-workload-response` |
| List | `…control.WPING` (all) / `…control.WPING.<workload_id>` (one) | — → `agent-list-workloads-response` |
| Undeploy | `…control.UNDEPLOY.<workload_id>` | `stop-workload-request` → `stop-workload-response` |
| Clone | `…control.CLONE.<workload_id>` | `clone-workload-request` → `clone-workload-response` |
| Update | `…control.UPDATE.<workload_id>` | `update-workload-request` → `update-workload-response` |
| Restart | `…control.RESTART.<workload_id>` | `restart-workload-request` → `update-workload-response` |
| Node ping / info / lameduck | `…control.PING[.<node_id>]`, `…control.INFO.<node_id>`, `…control.LAMEDUCK.<node_id>` | `node-ping-request`/`node-info-request`/`lameduck-request` → matching responses |

### AUCTION — find capacity

Request carries the desired `agent_type` and tags. Every node that has a
**healthy** registered nexlet of that type, satisfies every tag, and passes
its auctioneer (if configured) replies with a one-shot `bidder_id`, the
nexlet's `start_request_schema`, and its supported lifecycles. A bid is a
short-lived, TTL-expiring claim on that node — deploy against it promptly,
or re-auction. Expected observable: zero or more replies; no state changes
anywhere.

### ADEPLOY — deploy against a bid

Addressed to a `bidder_id` from a prior auction; only the node that issued
the bid (and still holds it un-expired) replies. The node validates the
`run_request` against the nexlet's schema, mints namespace-scoped NATS
credentials for the workload, generates the workload ID, and asks the nexlet
to start it. Expected observable: a `start-workload-response` carrying the
new workload ID; the workload appears in `WPING` listings; a
`WORKLOADSTARTED` event on `$NEX.FEED.<ns>.events…`; on a stateful node a
workload record is persisted (see *State semantics*) **after** the response
is sent — an immediately-following stateful operation should poll for the
record rather than assume it.

### WPING — list

Returns workload summaries aggregated from every agent on each node.
Read-only. Under the `system` namespace it returns all namespaces.

### UNDEPLOY — stop

The owning node asks the nexlet to stop the workload and replies with
`stopped: true|false`. The stop is **synchronous and honest**: `true` means
the nexlet confirmed the process/container is actually gone, not merely that
a signal was sent. Expected observable on success: the process or container
is gone (for containers: removed, not just stopped), the workload disappears
from listings, a `WORKLOADSTOPPED` event fires, and on a stateful node the
workload record is purged — the workload will **not** be resumed on the next
agent registration. On `stopped: false` nothing is purged and the reply's
message says why; retry is safe.

### CLONE — copy elsewhere

Fetches the live definition from the owning nexlet, then runs a fresh
auction + deploy for it (optionally with new tags, optionally stopping the
source afterwards). Expected observable: a **new** workload ID running the
same definition; the source is untouched unless a stop was requested.
Credentials are freshly minted for the clone — clones never share identity
with the source.

### UPDATE — replace in place

Replaces a running workload's definition **in place**: same workload ID,
same node, same namespace, same workload type (a namespace or type change is
rejected with 403; relocation is what `CLONE` is for). Order of operations,
and why it matters:

1. Validate the new `run_request` against the nexlet's schema.
2. Mint fresh credentials for the replacement instance.
3. **Persist the new definition first** (stateful nodes; compare-and-swap —
   a concurrent writer surfaces as `updated:false` "modified concurrently",
   never a silent overwrite).
4. Stop the running instance and **wait for confirmation**.
5. Only then start the replacement.

This ordering guarantees there is never a moment with two live instances of
the same workload (no dual writer), and that a crash mid-update on a
stateful node completes — never reverts — on the next agent registration.

Expected observable on `updated:true`: same workload ID, but a **new
process PID / new container ID**, a new start time in listings, one
`WORKLOADSTOPPED` + one `WORKLOADSTARTED` event, and the new definition on
file. On `updated:false` the message states exactly which stage stopped the
update and what happens next (see *Partial outcomes*).

### RESTART — bounce

Stops the workload and starts it again from its **last stored definition**,
with freshly minted credentials. If a previous update was interrupted
(stop unconfirmed, or replacement start failed), the stored definition is
already the new one — restart *finishes* that update rather than reviving
whatever happened to be running. On a node without `--state` (or for a
workload whose record was never written) there is nothing stored, so restart
replays the definition the nexlet is currently running — still a real
bounce, just with no definitional change.

Expected observable on `updated:true` — **with or without KV state**: same
workload ID, **new process PID / new container ID** (container *name* stays
the workload ID, so check the ID or start time, not the name), new start
time, stop+start event pair. A restart whose only expected effect is the
bounce (stateless, or stored == live) looks like a no-op in the workload's
output stream; verify it via the PID/container ID or the listing's start
time, not stdout.

## State semantics: with and without `--state`

| | stateless (default) | stateful (`--state`) |
|---|---|---|
| Deploy | runs; nothing recorded | runs; record persisted (post-response, create-only) |
| Node/agent restart | workloads are not resumed | every recorded workload is resumed on agent registration; a still-running instance is adopted, not duplicated |
| UNDEPLOY confirmed | instance gone | instance gone **and record purged** — no later resurrection |
| UPDATE | in-place replace; nothing survives a node restart | store-first + CAS; an interrupted update self-heals on the next registration |
| RESTART | replays the live definition | replays the stored definition |
| Record writers | — | all record writes are compare-and-swap; concurrent verbs on one workload resolve to exactly one winner, the loser reports `updated:false` |

## Partial outcomes (`updated:false`)

`UPDATE`/`RESTART` report self-healing partial outcomes as `updated:false`
with an explanatory message rather than as errors:

- **Stop unconfirmed** — the stop was dispatched but no nexlet confirmed it
  within the budget. The workload may still go down without a replacement.
  Stateful: the new definition is already on file and applies on the next
  agent registration. Stateless (or when the verb itself had created the
  record): nothing was stored, and the record the verb created is rolled
  back — so a workload a concurrent undeploy already removed can never be
  resurrected by an update racing it.
- **Replacement start failed** — the old instance is confirmed stopped,
  nothing is running. Stateful: heals on next registration. Stateless: the
  message says to redeploy.
- **Modified concurrently** — another writer changed the record between this
  verb's read and its write. Nothing was stopped or started; re-inspect and
  retry against the current record.

Clients must treat the `message` as operator-facing text, not as a parse
target; the stable signals are `updated` and the "stop unconfirmed" prefix.

## Credentials

Every deploy, clone, update, and restart mints **fresh** namespace-scoped
NATS credentials for the (new) instance; credentials are never carried over
from a previous generation. On stateful nodes the minted public user nkey is
persisted in the workload record's metadata (`nex_minted_nkey`) so a future
credential-revocation ("fencing") pass can target exactly the credential the
live instance holds.

## Verifying that a verb actually acted

Because workload IDs (and container names) are stable across update and
restart, "nothing changed" in a dashboard or log stream does not mean
nothing happened. The reliable signals, in order of strength:

1. Process PID (native) or container ID (container nexlets) — changes on
   every update/restart.
2. `workload list` start time — resets on every update/restart.
3. `$NEX.FEED.<ns>.events` — a `WORKLOADSTOPPED`/`WORKLOADSTARTED` pair per
   replacement.
4. On stateful nodes, the record in the node's KV bucket (`nex-<node_id>`)
   — new revision per update, purged on undeploy.
