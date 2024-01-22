[![Lint | Test | Build](https://github.com/synadia-io/nex/actions/workflows/build.yml/badge.svg)](https://github.com/synadia-io/nex/actions/workflows/build.yml)
![Release](https://github.com/synadia-io/nex/actions/workflows/release.yml/badge.svg)

# NATS Execution Engine
Leverage and extend your investment in NATS infrastructure to deploy functions and services, turning NATS into the ultimate platform for building distributed applications.

## Quickstart
The easiest way to get started with Nex is to check out our [Using Nex](https://docs.nats.io/using-nats/nex) section in the NATS documentation.

If you're already familiar with Nex and how it works, then you can get going quickly by installing the Nex CLI:

```
go install github.com/synadia-io/nex/nex@latest
```

_**Note** that an installation method that doesn't require Go is coming soon._

## Preflight Check

If you haven't already, please make sure that `/usr/local/bin` is in your path. 

The `nex node preflight` command is here to help you bootstrap your system and ensure that you have all of the elements you need to start and operate Nex nodes.

The pre-flight check requires a configuration file so it can perform the appropriate checks. If you don't already have a configuration file, then Nex's `preflight` command will generate a new one for you in `./config.json`, as shown below:

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

Run the following command to get everything set up using the above defaults:

```
$ nex node preflight
```

## Nex Components
Nex is made up of the following components

* [agent](./agent) - Agent that runs inside a Firecracker VM, responsible for running untrusted workloads. Not something end users need to interact with.
* [fc-image](./agent/fc-image/) - Tools for building the rootfs (ext4) file system for use in firecracker VMs
* [node](./internal/node) - Service running on a NEX node. Exposes a control API, starts/stops firecracker processes, communicates with the agent inside each process.
* [nex](./nex) - CLI for communicating with NEX nodes
* [ui](./ui) - User interface for viewing the status of NEX nodes in a web browser

## Contributing
For information on how to contribute to Nex, please read our [contributing](./CONTRIBUTING.md) guide.
