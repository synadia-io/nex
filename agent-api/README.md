# Agent API
This is the API used for communication between the agent (process running inside the firecracker VM) and the host (`nex-node`). This API contains operations to subscribe to logs and events, as well as health query and, of course, a function to start and run a workload.

Most of this package is generated from protobufs, but there is also a client that can be used to communicate with `nex-agent`s.