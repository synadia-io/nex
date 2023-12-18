# NATS Execution Engine
Turn your NATS infrastructure into a distributed workload deployment and execution engine.

* [nex-agent](./nex-agent) - Agent that runs inside a Firecracker VM, responsible for running untrusted workloads. Not something end users need to interact with.
* [nex-node](./nex-node) - Service running on a NEX node. Exposes a control API, starts/stops firecracker processes, communicates with the agent inside each process.
* [nex-cli](./nex-cli) - CLI for communicating with NEX nodes
* [control-api](./control-api/) - The API for communicating with and remotely controlling NEX nodes
* [agent-api](./agent-api/) - Data types and other API data for protocol used between `nex-node` and `nex-agent` across the firecracker boundary. This is an internal API unlikely to be of interest to anyone 
* [fc-image](./fc-image/) - Tools for building the rootfs (ext4) file system for use in firecracker VMs
other than contributors.

To get up and running, follow the [Getting Started](./docs/getting_started.md) guide.