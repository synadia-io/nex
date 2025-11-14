---
title: Running Nex Nodes
description: Launching and operating Nex nodes with local or remote nexlets
sidebar_position: 2
---

# Running Nex Nodes

A Nex node coordinates auctions, workload placement, and lifecycle orchestration across the nexlets registered with it. This guide walks through preparing your environment, starting a node for local development or remote operation, and using the CLI to observe and drain capacity safely.

## Before You Start

- **Build the CLI**: From `synadia-labs/nex`, run `task nex` (or `go build ./cmd/nex`) and put the resulting `nex` binary on your `PATH`.
- **Verify NATS access**: The CLI connects through your NATS credentials or context. Make sure the account can publish and subscribe to `$NEX.*`. If you rely on a NATS context, configure it ahead of time ([docs](https://github.com/nats-io/nats.docs/blob/master/using-nats/nats-tools/nats_cli/README.md#nats-contexts)):
  ```bash
  nats context add nex-dev --server nats://demo.nats.io:4222 --creds /path/to/account.creds
  nats context use nex-dev
  nats context info nex-dev
  nats ping --context nex-dev
  ```
  The `info` and `ping` commands confirm authentication and connectivity before you start the node.
- **Have at least one nexlet ready**: The native nexlet starts automatically unless you disable it. Additional nexlets (custom or third-party) must be accessible on the node host or able to register remotely.
- **Collect node identity material**: Generate a server seed (`nkeys create --type server`) and optional curve seed for encrypted traffic if you plan to reuse the node identity between restarts.
- **Choose a configuration strategy**: The CLI reads JSON config from `/etc/nex/config.json`, `~/.config/nex/config.json`, or `./config.json`. You can also pass `--config path/to/config.json` explicitly.

## Quickstart: Local Development Node

1. Ensure your NATS CLI context is authenticated (see setup above) and actively selected: `nats context use nex-dev`.
2. Launch a node with auto-started native nexlet:
   ```bash
   nex --namespace system node up \
     --nats.context nex-dev \
     --node-name lab-node \
     --tags arch=$(uname -m) \
     --allow-remote-agent-registration
   ```
3. The CLI logs include the generated node ID and registration of the native nexlet. Press `Ctrl+C` to stop the node, or run `nex node lameduck --node-id <id>` from another terminal to drain gracefully before shutdown.

The `--namespace system` global flag targets the administrative namespace used for node operations. When you set defaults in a config file, include the same flag so subsequent commands inherit it automatically.

## Understanding Node Options

### Identity and Encryption

- `--node-seed` supplies a stable node identifier. Omit it for an ephemeral development node.
- `--node-xkey-seed` controls the curve key used to encrypt secrets exchanged with nexlets. Generate one if you need deterministic secrets across restarts.
- The CLI generates both values when unspecified.

### Connecting to NATS

- Use `--nats.context` for existing NATS CLI contexts, or supply servers/credentials manually (`--nats.servers`, `--nats.nkey`, `--nats.creds-file`, etc.).
- To embed an internal NATS server, pass `--inats-config /path/to/nats.conf`. The node will start the server and use the provided credentials to mint connection data for itself and spawned nexlets.
- `--nats.conn-name` tags the underlying connection for easier monitoring.

### Managing Nexlets

- By default the node starts the native nexlet via the Go SDK runner. Disable it with `--disable-native-start` if you only use remote nexlets.
- Add additional local nexlets with repeated `--agents.uri`, `--agents.argv`, and optional `--agents.env` flags (or the matching JSON fields). Each entry describes a binary the node spawns and supervises. URIs can be `file://`, `nats://`, or plain filesystem paths.
- `--agent-restart-limit` caps automatic restarts for supervised nexlets (default `3`). The node stops trying after it hits the limit.
- Use `--allow-remote-agent-registration` when nexlets run on other machines. Remote nexlets connect to NATS using credentials minted by the node. See “Credential minting” below.

### Credential Minting for Workloads and Remote Nexlets

A node must issue scoped NATS credentials so workloads and remote nexlets can communicate securely. Choose one of three strategies:

1. **Signing key + root account**: Provide an account signing seed (`--issuer-signing-key`) and the corresponding root account public key (`--issuer-signing-key-root-account`). The node signs user JWTs.
2. **User NKEY**: Provide a user NKEY (`--issuer-nkey`) and seed (`--issuer-nkey-seed`). The node clones and restricts that identity per workload.
3. **Full access (development only)**: When neither option is provided, the node falls back to `FullAccessMinter`, which issues unscoped credentials. Use this only for local experiments.

Remote nexlets fail to register if the node cannot mint credentials. Double-check that you supplied a valid signing configuration before enabling `--allow-remote-agent-registration`.

### Persistence and Recovery

- `--state kv` (or `"state": "kv"` in JSON) enables persistence via a NATS Key-Value bucket named `nex-<node_id>`. The node restores workloads after restarts and supports disaster recovery. The empty string keeps everything in-memory.
- Keep the KV bucket in the same JetStream domain the node uses, or specify `--nats.jsdomain`.

### Logging

- Global logger flags (`--logger.level`, `--logger.target`, `--logger.with-pid`, etc.) apply to both node logs and workload log forwarding.
- `--show-workload-logs` disables the default filter that hides per-workload logs from the node console.

## Using Configuration Files

Persist reusable options in a JSON file and pass `--config path/to/config.json` (or rely on one of the default locations). Keys mirror CLI flags with dashes converted to underscores.

```json title="Sample config.json"
{
  "namespace": "system",
  "node": {
    "up": {
      "node_seed": "SNAXXX...",
      "node_xkey_seed": "XAXXX...",
      "node_name": "prod-node-1",
      "nexus": "regional-cluster",
      "allow_remote_agent_registration": true,
      "agent_restart_limit": 5,
      "state": "kv",
      "tags": {
        "region": "lab",
        "arch": "amd64"
      },
      "agents": [
        {
          "uri": "file:///opt/nex/agents/native",
          "argv": ["run"]
        },
        {
          "uri": "file:///opt/nex/agents/custom",
          "argv": ["run"],
          "env": {
            "NEX_RUNTIME_SOCKET": "/run/custom.sock"
          }
        }
      ],
      "issuer_signing_key": "SA...SIGNING...",
      "issuer_signing_key_root_account": "ACCT...",
      "inats_config": "/etc/nats/server.conf"
    }
  },
  "nats": {
    "context": "nex-prod",
    "timeout": "5s"
  },
  "logger": {
    "level": "info",
    "target": ["std"]
  }
}
```

Run `nex --config ./config.json --check` (or append `--check` to any command) to view the resolved configuration without starting the node.

### Field Highlights

| Field | Description |
| --- | --- |
| `node_seed`, `node_xkey_seed` | Persisted identities for stable node IDs and encrypted nexlet channels. |
| `allow_remote_agent_registration` | Accept nexlets that connect after startup. Required for remote runtimes. |
| `agents` | Local nexlets the node supervises. Each entry maps to a binary plus arguments/environment. |
| `state` | Persistence backend (`""` for volatile, `"kv"` for NATS Key-Value). |
| `issuer_signing_key` + `issuer_signing_key_root_account` / `issuer_nkey` + `issuer_nkey_seed` | Credential minter inputs. Supply exactly one pair. |
| `tags` | Placement metadata exposed via `nex node list` and used in workload scheduling. Avoid reserved prefixes (`nex.`). |

## Operating Running Nodes

### List Nodes

```bash
nex --namespace system node list
```

The table shows each node’s nexus, ID, version, uptime, current state (`RUNNING`, `LAMEDUCK`, etc.), and the number of registered agents. Append `--filter key=value` to limit results by tag.

### Inspect a Node

```bash
nex --namespace system node info <node_id> --full
```

The detailed view includes the node’s tags, XKey, uptime, version, and per-agent heartbeat information (health stoplight, supported lifecycles, running workload count, and last heartbeat timestamp).

### Enter Lame Duck Mode

```bash
nex --namespace system node lameduck --node-id <node_id> --delay 2m
```

The node stops accepting new workloads immediately and begins shutting down existing workloads after the delay expires. Use tags (`--tag key=value`) to drain entire pools at once.

### Shutdown and Restart

- Press `Ctrl+C` in the session that started the node to trigger a graceful shutdown. The CLI traps the signal and calls `nex.Shutdown()`.
- Combine `node lameduck` with `Ctrl+C` for production nodes so workloads finish cleanly.
- When using KV state, a restart with the same `node_seed` recovers prior workload assignments and replays `StartWorkload` for each entry.

## Troubleshooting Tips

- **NATS connection errors**: Confirm your chosen `--nats.context` or credentials file can reach the cluster and includes JetStream access if you enable persistence.
- **Remote nexlet registration fails**: Verify the node logs mention a credential minter. Without signing material the node issues broad credentials that many secured NATS deployments reject.
- **Agent crash loops**: Increase `--agent-restart-limit` during debugging, and inspect the per-agent logs (enable `--show-workload-logs` or tail the agent’s own output).

With these foundations in place you can add custom nexlets (see “Creating Custom Nexlets”) and build higher-level automation on top of the Nex node API.
