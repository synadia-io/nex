---
# These are optional metadata elements. Feel free to remove any of them.
status: "proposed"
date: 2024-SEP-11
decision-makers: Nex team
---

# Decoupled and Multitenant Agents

## Context and Problem Statement

The current state of Nex assumes that agents are _not_ multitenant and _are_ tightly coupled. Every new workload started gets its own private agent and, in many cases, its own private **firecracker** microVM, creating massive inefficiencies at scale, especially when dealing with hundreds or thousands of functions. A microVM per function is excessive when the function runtimes typically have their own sandbox and security isolation.

The problem is that the current architecture reveals its limitations once large numbers of workloads are started on a single Nex node.

<!-- This is an optional element. Feel free to remove. -->
## Decision Drivers

* Resource usage and footprint size
* Tightly coupled dependencies - Agent dependencies become Nex dependencies because everything is wrapped up in a single `nex-agent` binary. For example, an agent requiring CGO to use V8 then means the `nex-agent` binary must have this requirement for non-V8 workloads as well.
* Performance
* Inconsistency or unreliability of **handshakes** with pooled, asynchronously started agents
* Proliferation of potential race conditions

## Considered Options
We considered the idea of using `mime` types to identify the type of workload being executed but ultimately decided that would add complexity rather than move it. We thought the mime types might help with the idea of having JavaScript's agent use more than one `.js` file, but the agent can detect if the artifact is an archive or not, making the use of complicated mime type hierarchies unnecessary.

## Decision Outcome

The outcome of this decision was to extract all of the agent functionality from the existing `nex-agent` process, create a single agent per workload type, and then have each of those independent agents manage multi-tenancy in the manner of their own choosing.

<!-- This is an optional element. Feel free to remove. -->
### Consequences

* 👍 Nex node no longer needs compiled-in dependencies used by individual workload types
* 👍 Multitenant agents mean there is significantly less per-workload overhead, especially on function types
* 👍 Moving a large amount of code to the agents can dramatically cut down on the complexity of the core Nex node codebase
* Makes `preflight` a little bit more complicated as it now needs to download agents for first-party workload types
* Creates breaking changes pretty much everywhere, making it so at least the first two (JavaScript, Native) agents would have to be built along with the Nex refactor.

## Detailed Design
In this new design, the `nex` binary is distributed on its own. During the `preflight` command we can download all of the relevant agents the same way we download the `firecracker` binary and the CNI plugin files.

**All agents are multi-tenant**

The Nex node configuration file will change slightly to accommodate this. Inside the `workload_types` configuration object, instead of having just a list of the name of each supported workload type, we will now have configuration per type:

```json
{
    ...
    "workload_types": [
        {
            "name": "native",
            "agent_path": "/opt/agents/native",
            "argv": [...],
            "env": {
                "a": "b"
            }                     
        },
        {
            "name": "javascript",
            "agent_path": "/opt/agents/javascript"           
        }    
    ] 
}
```
Note that if an `agent_path` is not supplied for the supported workload type, then Nex will look for the agent at `./agents/{name}`, relative to the Nex startup directory. Running `preflight` against a configuration file with first-party workload types defined can automatically download those agents and put them in the appropriate location.

In some cases, an agent may support artifacts that are either regular files or **archives**. When this occurs (e.g. in the case of the `JavaScript` agent, which can run a single `.js` file or the contents of a tarball), it is up to the agent to detect whether the artifact is an archive or not and to deal with it properly.

Poorly formed or invalid archives should be rejected by the agent in response to the start request.

All agents are spawned by the `Nex` node process at startup and must _not_ be started manually outside the Nex node process (e.g. they are _not_ sidecars). Each agent will be supplied with the following environment variables which allow it to connect to the Nex internal NATS "bus":

* `INT_NATS_HOST` - Host (IP) of the Nex internal NATS server. This will almost always be `0.0.0.0` or `127.0.0.1`, and is determined by the node configuration file.
* `INT_NATS_PORT` - Nex starts the internal NATS server on a _random_ port. This environment variable contains that port number.
* `INT_NATS_NKEY_SEED` - The **nkey _seed_** used by the agent to connect to the internal server. This seed is dynamically generated every time the Nex node starts.

Agents can also be passed environment variables and command line arguments via the `env` and `argv` fields in the agent's section of the node config.

All agents are started when the Nex node starts. The Nex node will not accept workload start requests until the agents have finished the startup registration sequence.

### Native Executable Agent
The **native executable agent** is responsible for running single-binary executables on whatever platform the Nex node resides. The workload start request sent to this agent will contain information about whether the executable is to be wrapped in a microVM.

In the case where a microVM is called for, the native agent (_not the Nex node_) will be responsible for building a root file system dedicated to running that single binary. It will then launch a new microVM instance (e.g. `firecracker`) to run that executable.

The native executable agent is responsible for the life cycle of all child processes, whether they are wrapped in a microVM or not. It must be able to gracefully terminate all of those processes on request.

The template used for building the on-demand root file system is configurable in the appropriate workload type section of the node configuration.

The native agent is responsible for supplying the executable workload with enough information to connect to host services. Where the function-type agents expose a host services proxy, the native workloads are allowed to connect directly to the indicated URL/creds.

### JavaScript Agent
The JavaScript agent is responsible for executing individual _functions_. These functions are designed to be short-lived and the agent can choose to reclaim resources from those functions whenever it deems necessary.

The agent can execute a single JavaScript file (`.js`) or it can take an archive file as the artifact, where it will extract all `.js` files from the archive and run them in a single context, provided there remains only one exported `run` function.

The JavaScript agent is responsible for exposing host services to each function. The agent must also ensure that no two workloads share the same execution context.

### WebAssembly Agent
The WebAssembly agent is responsible for executing WebAssembly _components_. We no longer support running the classic modules. All executable WebAssembly artifacts will be components with their interfaces defined using the `wit` convention (and likely the appropriate `wit-bindgen` tooling).

The WebAssembly agent is responsible for exposing host services to each function, proxying access to the host services API exposed on the internal NATS server.

### OCI Agent
The OCI agent will be responsible for running images from an indicated container registry. This effectively gives Nex nodes the ability to schedule and start docker images.

Information on how to connect to the appropriate host services URL/creds will be provided to the workload. OCI workloads are considered long-lived services and as such do not require a host services proxy.

## New Agent Protocol
Every agent is required to adhere to this agent protocol. The agents _must_ connect to the internal NATS server embedded in the Nex node host and this is the _only_ way that agents and Nex nodes communicate with each other other than when the Nex node initially spawns an agent.

Note that in the table below, _direction_ is relative to the _agent_. `hostint.>` is an export from the Nex node account, while `agentint.>` is an export from the agent's user/account. All communications in the table below happens on the internal connection, _agents should not communicate with any NATS servers other than the Nex node host_.

| Subject | Direction | Description |
|--|--|:--|
| `hostint.{workload-type}.register` | **request** | Registers the agent information with the Nex node host. Here `workload-type` is the name of the workload type this agent handles, e.g. `native` or `javascript` |
| `hostint.{workload-type}.logs` | **publish** | Emits logs from running workloads for the Nex node to capture and potentially re-broadcast |
| `hostint.{workload-type}.events.{event-type}` | **publish** | Emits a cloud event from the agent to the Nex node |
| `hostint.hostservices.<service>.<method>` | **request** | When proxying host services calls, the agent will use this subject. The calling workload will be indicated with `x-agent-workloadid`, `x-agent-workloadtype`, and `x-agent-namespace` headers. The raw payload is passed directly. For example, a JavaScript function making a key-value `get` request would make a request on `hostint.hostservices.keyvalue.get`. Additional headers for e.g. metrics/tracing or for specific host services can also be passed. |
| `agentint.{workload-type}.startworkload` | **reply** | Handles a request from the Nex node to start a workload |
| `agentint.{workload-type}.stopworkload` | **reply** | Handles a request from the Nex node to stop a workload | 
| `agentint.{workload-type}.workloads.get` | **reply** | Handles a request from the Nex node for a list of all running workloads and their state, if applicable | 
