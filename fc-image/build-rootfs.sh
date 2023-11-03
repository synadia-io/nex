#!/bin/bash

set -xe

dd if=/dev/zero of=rootfs.ext4 bs=1M count=100
mkfs.ext4 rootfs.ext4
mkdir -p /tmp/my-rootfs
mount rootfs.ext4 /tmp/my-rootfs

docker run -i --rm \
    -v /tmp/my-rootfs:/my-rootfs \
    -v "$(pwd)/../nex-agent/cmd/nex-agent/nex-agent:/usr/local/bin/agent" \
    -v "$(pwd)/openrc-service.sh:/etc/init.d/agent" \
    alpine sh <setup-alpine.sh

umount /tmp/my-rootfs

# rootfs available under `rootfs.ext4`