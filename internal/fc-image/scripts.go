package rootfs

const (
	copy_fs = `#!/bin/sh
set -xe

for d in bin etc lib root sbin usr; do tar c "/$d" | tar x -C /tmp/rootfs; done
for dir in dev proc run sys var tmp; do mkdir /tmp/rootfs/${dir}; done

chmod 1777 /tmp/rootfs/tmp
mkdir -p /tmp/rootfs/home/nex/
chown 1000:1000 /tmp/rootfs/home/nex/`
)
