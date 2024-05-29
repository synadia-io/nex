package nexnode

const defaultNodeConfig string = `{
    "default_resource_dir":"/tmp/nex",
    "machine_pool_size": 1,
    "internal_node_host":"10.10.10.1",
    "cni": {
        "network_name": "fcnet",
        "interface_name": "veth0",
        "subnet": "10.10.10.0/24"
    },
    "machine_template": {
        "vcpu_count": 1,
        "memsize_mib": 256
    },
    "tags": {
        "simple": "true"
    }
}`

const defaultNoSandboxNodeConfig string = `{
    "default_resource_dir":"/tmp/nex",
    "machine_pool_size": 1,
    "internal_node_host":"10.10.10.1",
    "cni": {
        "network_name": "fcnet",
        "interface_name": "veth0",
        "subnet": "10.10.10.0/24"
    },
    "machine_template": {
        "vcpu_count": 1,
        "memsize_mib": 256
    },
    "tags": {
        "simple": "true"
    },
    "no_sandbox": true
}`
