![go build status](https://github.com/ConnectEverything/nex/actions/workflows/go.yml/badge.svg)

# NATS Execution Engine
Turn your NATS infrastructure into a distributed workload deployment and execution engine.

* [nex-agent](./nex-agent) - Agent that runs inside a Firecracker VM, responsible for running untrusted workloads. Not something end users need to interact with.
* [nex-node](./nex-node) - Service running on a NEX node. Exposes a control API, starts/stops firecracker processes, communicates with the agent inside each process.
* [nex-cli](./nex-cli) - CLI for communicating with NEX nodes
* [control-api](./control-api/) - The API for communicating with and remotely controlling NEX nodes
* [agent-api](./agent-api/) - Data types and other API data for protocol used between `nex-node` and `nex-agent` across the firecracker boundary. This is an internal API unlikely to be of interest to anyone 
* [fc-image](./fc-image/) - Tools for building the rootfs (ext4) file system for use in firecracker VMs
other than contributors.


## Quickstart

`GOPRIVATE=github.com/ConnectEverything/nex go install github.com/ConnectEverything/nex/nex@main`

The `nex node preflight` command is here to help you bootstrap your system.  

First, we need to create a few directories it is expecting 
```
mkdir -p /opt/cni/bin
mkdir -p /etc/cni/conf.d
mkdir -p /tmp/wd
```
You will also need to make sure that `/usr/local/bin` is in your path

Once those things are confirmed, use this configuration file

```json
{
    "default_resource_dir":"/tmp/wd",
    "machine_pool_size": 1,
    "cni": {
        "network_name": "fcnet",
        "interface_name": "veth0"
    },
    "machine_template": {
        "vcpu_count": 1,
        "memsize_mib": 256
    },
    "tags": {
        "simple": "true"
    }
}
```

along with the below command 

`sudo nex node preflight --config config.json`

There is a `--force` flag if you do not want to be prompted to install the missing dependencies
