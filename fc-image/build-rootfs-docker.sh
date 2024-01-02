#!/bin/bash

set -xe

# adding docker to the rootfs bumps size to require at least 600MB. Increase this if
# you find you run out of disk space when executing a workload
dd if=/dev/zero of=rootfs-docker.ext4 bs=1M count=600
mkfs.ext4 rootfs-docker.ext4
mkdir -p /tmp/my-rootfs
mount rootfs-docker.ext4 /tmp/my-rootfs

docker run -i --rm \
    -v /tmp/my-rootfs:/my-rootfs \
    -v "$(pwd)/../nex-agent/cmd/nex-agent/nex-agent:/usr/local/bin/agent" \
    -v "$(pwd)/openrc-service.sh:/etc/init.d/agent" \
    alpine sh <setup-alpine.sh

umount /tmp/my-rootfs

# rootfs available under `rootfs.ext4`