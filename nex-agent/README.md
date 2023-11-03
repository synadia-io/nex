# NEX Agent
The NATS execution engine agent is a process that is built into the root FS used by Firecracker images. This process launches are startup and communicates with the `nex-node` host process to coordinate running workloads, exchanging streams of events and log entries, etc.

It's worth noting that every `nex-agent` exists within _one_ firecracker VM and will only ever manager _one_ workload. 