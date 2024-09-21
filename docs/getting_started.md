# Getting Started with Nex

Nex is a suite of tools for deploying applications and services into a NATS environment. Anywhere messages can reach, so too can your deployments. Nex is a completely separate add-on to NATS and doesn't require any custom forks of existing software.

## Installation

The first thing you'll want to do is install the `nex` binary and compile the `nex-agent` binary (required to create a Root FS, discussed shortly).

At the moment since Nex is still in its infancy, we don't have an automated release or installation process. This means that you'll want to clone this repository, go to the root directory, and run `task build`. If you don't have (or want to) task installed, you can always read the [Taskfile.yml](../Taskfile.yml) file and manually run each of the build commands, e.g.

```
go build -tags netgo -ldflags '-extldflags "-static"'
```

Consult the appropriate documentation if you're building workloads in another language like Rust, C, or Zig that have excellent static linking support.

We will have an installation process available soon. We will also have official release images for the root filesystem, letting you skip the next step.

### Building a Root File System

Every firecracker virtual machine is a melding of a Linux kernel binary and a root file system. In our case, the root file system contains an [agent](../agent/) configured to start automatically during boot time. To make sure this agent is in the root file system, follow the instructions in the [fc-image](../agent/fc-image/) directory to create a `rootfs.ext4` file.

You should locate and download whichever Linux kernel binary you think you'll want. We do all our testing with the most recent stable Linux kernel configured with the defaults.

### Install CNI Prerequisites

The [Nex node](../internal/node/) process creates firecracker virtual machines and provisions CNI networks according to CNI configuration files. You'll need to make sure that whatever plugins you need are installed in the `/opt/cni/bin` directory. We currently use the following plugins in the default CNI configuration:

- `ptp`
- `tc-redirect-tap`
- `host-local`

`ptp` and `host-local` are part of the [CNI plugins](https://github.com/containernetworking/plugins) bundle. `tc-redirect-tap` is a plugin managed and released by [AWS](https://github.com/awslabs/tc-redirect-tap)

### Preflight Checklist

Nex has a few interesting requirements because it's essentially a distributed scheduling framework. To make this easier for you during setup, you can run `nex node preflight` and specify the path to an intended machine configuration. It will then go looking for all of the appropriate plugins, config files, etc. If you don't already have a CNI configuration for your device, it'll create one for you.

## Starting a Nex Node

Nex is an opt-in, completely separate add-on to NATS. It doesn't require any custom forks, distributions, or servers. All you need to do is start up a nex node process as an "empty vessel" awaiting workloads via remote commands.

To start a nex node, you just need to run the following command:

```
$ sudo nex node up --config=simple.json
```

This will start a nex node process using the data found in `simple.json` to configure the node host as well as define the cookie cutter template used for producing new firecracker virtual machines.

A simple configuration file looks like this:

```json
{
  "kernel_path": "/home/kevin/lab/firecracker/vmlinux-5.10",
  "rootfs_path": "/home/kevin/lab/firecracker/rootfs.ext4",
  "machine_pool_size": 1,
  "cni": {
    "network_name": "fcnet",
    "interface_name": "veth0"
  },
  "machine_template": {
    "vcpu_count": 1,
    "memsize_mib": 256
  }
}
```

The `kernel_path` needs to be a kernel binary that you've downloaded and made available. The `rootfs_path` is the path to the root file system you created in the previous step. There are some extra configuration options that you can use to do things like limit the issuers capable of starting workloads and even defining rate limit buckets for the spawned virtual machines. Examples of those can be found in [examples](../examples/).

You almost definitely need to use `sudo` to start this because of the changes to networking and system calls made by firecracker and the firecracker SDK. In production deployments, you might want to create a special `nex` user that can do only the things required by firecracker.

Once this is running, in another terminal, run:

```
$ nex node ls
```

This will attempt to discover all running nodes. If you don't see your node in this list, go back over the previous steps and make sure all of the networking is configured properly and you have a NATS server running, etc.

## Running Workloads

At this point, you should have:

- A version of the [nex binary](../nex/), `nex`.
- A running node host that you launched in the previous section.
- Verified the node host is running via `nex node ls`

A workload, as far as nex is concerned, is a statically linked, 64-bit, Linux elf binary. For this guide, we'll assume that we're using the [echoservice](../examples/echoservice/) workload demo. The is absolutely nothing special about this application. It doesn't use any custom SDKs, doesn't require any custom libraries. The only dependency it has is that the `NATS_URL` environment variable be set.

Workloads also come with some security. Each workload must be signed by an issuer. This concept should be familiar to anyone who has used NATS' decentralized authorization before. Additionally, each workload's environment variables are encrypted using an **xkey**.

Additionally, all workloads must currently be stored in a JetStream Object Store.

`nex` will run workloads either in "developer mode" or in a more rigid, production-style manner.

### Running Workloads with Developer Mode

When running workloads locally, we don't need to be bogged down by the details that only apply when running in production. In developer mode, `nex` will automatically create an issuer key, an encryption xkey, and even upload the raw binary to an object store on your behalf. It will even locate all possible deployment target nodes and pick the first one available for you.

This means you can use developer mode to "fire and forget" launch workloads easily. Just run a command like this one:

```
nex devrun ./myworkload nats_url=nats://1.2.3.4:4222
```

The important things to note here are the use of `devrun` instead of `run`, and the value of the `nats_url` variable (which will be translated to the `NATS_URL` environment variable available to the running workload). The NATS URL being passed here is the address of a NATS server that is _accessible to_ the firecracker VM. It is **NOT** the address of the _internal NATS_ server used for private communications between host and VM.

So let's say I have a DNS entry for my lab NATS server and I'm using IP masquerade (the default) so that my firecracker VMs have the same network access as the host. I can then pass a URL like this:

```
nex devrun ./myworkload nats_url=nats://lab:4222
```

This works because we also dynamically generate a `resolv.conf` file inside the firecracker VM.

If you run `devrun` multiple times in a row, `nex` will actually delete the previous version of the workload, stop the previously running workload machine, and start the new one. This means that you can basically hit "up arrow" after you've done a static build and your most recent binary will be running.

### Running Workloads in Production

In production you want to be able to make start workload requests that have no assumptions or defaults, and you need to specify everything explicitly. For that, you'll use `nex run` as shown below:

```
$ nex run --xkey=./keys/publisher.xk \
   --issuer=./keys/issuer.nk \
   --name=echoservice \
   --description='This is the echo service' \
   nats://MYFILES/echoservice \
   Nxxxxxxxxxxxxxxxx \
   nats_url=nats://devlab:4222
```

This will attempt to run the workload stored in object store `MYFILES` under the key `echoservice` on the nex node `Nxxxxxxxxxxxxxxxx`.

If you're using the echo service from our examples, then when you run `nats micro ls` you'll actually see the instance of the service running inside a nex node. If you issue another run command (not `devrun`), you'll quickly see a second instance of that service running.

### Observing Workloads

You can monitor a stream of logs and events for running workloads. These events and logs are published on `$NEX.events.*` and `$NEX.logs.>` respectively. However, there's a more user-friendly way to monitor this using `nex logs` or `nex events`.
