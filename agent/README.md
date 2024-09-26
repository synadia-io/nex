# Nex Agent

The NATS Execution Engine **Agent** is a process that is built into the root FS used by Firecracker images. This is never run directly and
usually only important to contributors.

This process is configured as an OpenRC startup service so it's automatically running once the firecracker image reaches steady state (which only takes a ms or so).

Once running, the agent process uses the metadata available from MMDS to connect via NATS to the node host, where it can receive workload instructions and publish logs and events.

It's worth noting that every `nex-agent` exists within _one_ firecracker VM and will only ever manager _one_ workload.
