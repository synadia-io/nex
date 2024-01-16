# NATS Execution Engine Node
The NATS execution engine **node** service runs as a daemon on a host where it responds to control commands, starting, stopping, and monitoring workloads. 

## Requirements
The following is a (potentially incomplete) list of the things you'll need in order to be able to run the node application.

* An accessible copy of the `firecracker` binary _in the path_ of the node service
* A linux kernel
* A root file system with [nex-agent](../nex-agent/) configured as a boot service.
* CNI plugins installed (at least) to `/opt/cni/bin`
    * `host-local`
    * `ptp`
    * `tc-redirect-tap`
* A CNI configuration file at `/etc/cni/conf.d/fcnet.conflist`

## Running and Testing
First and foremost, the `nex node` application makes use of the firecracker SDK to start VMs, which is an act that requires a number of elevated privileges. When running locally for testing, it's easy enough to just `sudo nex-node ....` but in production, you might want to craft a bespoke user and group that can do just enough to make firecracker happy.

Because `nex node` utilizes firecracker VMs, ⚠️ _it can only run on 64-bit Linux_ ⚠️. The workloads you submit to execute on `nex node` must be _statically linked_, freestanding Linux elf binaries. The node service will reject dynamically-linked or non-elf binaries.

As you run this and create warm VMs to reside in the pool, you'll be allocating IP addresses from the firecracker network CNI device (defaults to `fcnet`). This means that eventually you'll run out of IP addresses during development. To clear them, you can purge your `/var/lib/cni/networks/{device}` directory. This will start IP allocation back at `.2` (`.1` is the host).

Additionally, while running and testing this you may end up with a large amount of idle/dormant `veth` devices. To remove them all, you can use the following script:

```bash
for name in $(ifconfig -a | sed 's/[ \t].*//;/^\(lo\|\)$/d' | grep veth)
do
    echo $name
    sudo ip link delete $name
done
```

⚠️ If you're working in a contributor loop and you modify the rootfs after some firecracker IPs have already been allocated, it can potentially wreck the host networking and you'll notice
the agents suddenly unable to communicate. If this happens, just purge the IPs and the `veth` devices to start over fresh.

## Configuration
The `nex node` service needs to know the size and shape of the firecracker machines to dispense. As a result, it needs a JSON file that describes the cookie cutter from which VMs are stamped. Here's a sample `machineconfig.json` file:

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
    },
    "requester_public_keys": []
}
```

This file tells `nex node` where to find the kernel and rootfs for the firecracker VMs, as well as the CNI configuration. Finally, if you supply a non-empty value for `requester_public_keys`, that will serve as an allow-list for public **Xkeys** that can be used to submit requests. XKeys are basically [nkeys](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/nkey_auth) that can be used for encryption. Note that the `network_name` field must match _exactly_ the `{network_name}.conflist` file in `/etc/cni/conf.d`.

## Reference
Here's a look at the `fcnet.conflist` file that we use as a default. This gives each firecracker VM access to whatever the host can access, and allows the host to make inbound requests. Regardless of your CNI configuration, the agent process _must_ be able to communicate with the host node via the internal NATS server (defaults to running in port `9222`).

```json
{
  "name": "fcnet",
  "cniVersion": "0.4.0",
  "plugins": [
    {
      "type": "ptp",
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "subnet": "192.168.127.0/24",
        "resolvConf": "/etc/resolv.conf"
      }
    },
    {
      "type": "tc-redirect-tap"
    }
  ]
}
```
If you want to add additional constraints and rules to the `fcnet` definition, consult the CNI documentation as we use just plain CNI configuration files here.

## Observing Events
You can monitor events coming out of the node and workloads running within the node by subscribing on the following subject pattern:

```
$NEX.events.{namespace}.{event_type}
```

The payload of these events is a **CloudEvent** envelope containing an inner JSON object for the `data` field.

## Observing Logs
You can subscribe to log emissions without console access by using the following subject pattern:

```
$NEX.logs.{namespace}.{host}.{workload}.{vmId}
```

This gives you the flexibility of monitoring everything from a given host, workload, or vmID. For example, if you want to see
an aggregate of all logs emitted by the `bankservice` workload throughout an entire system, you could just subscribe to:

```
$NEX.logs.*.*.bankservice.*
```
