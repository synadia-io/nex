package main

const (
	openrc_service = `#!/sbin/openrc-run

name=$RC_SVCNAME
description="Nex Agent"
supervisor="supervise-daemon"
command="/usr/local/bin/agent"
pidfile="/run/agent.pid"
command_user="nex:nex"
output_log="/home/nex/nex.log"
error_log="/home/nex/err.log"

depend() {
	after net.eth0
}`

	setup_alpine = `#!/bin/sh

set -xe

chmod +x /usr/local/bin/agent
chmod +x /etc/init.d/agent

apk add --no-cache openrc
apk add --no-cache util-linux
apk add --no-cache curl

ln -s agetty /etc/init.d/agetty.ttyS0
echo ttyS0 >/etc/securetty
rc-update add agetty.ttyS0 default

echo "root:root" | chpasswd

addgroup -g 1000 -S nex && adduser -u 1000 -S nex -G nex
curl https://binaries.nats.dev/nats-io/natscli/nats@latest | sh
./nats context save nhg_tokens -s 192.168.127.1

rc-update add devfs boot
rc-update add procfs boot
rc-update add sysfs boot

# This is our script that runs nex-agent
rc-update add agent boot

for d in bin etc lib root sbin usr; do tar c "/$d" | tar x -C /tmp/rootfs; done
for dir in dev proc run sys var tmp; do mkdir /tmp/rootfs/${dir}; done

chmod 1777 /tmp/rootfs/tmp
mkdir -p /tmp/rootfs/home/nex/
mkdir -p /tmp/rootfs/home/nex/.config/nats/context
mv /root/.config/nats/context/* /tmp/rootfs/home/nex/.config/nats/context/
cat /tmp/rootfs/home/nex/.config/nats/context/*

chown 1000:1000 /tmp/rootfs/home/nex/`
)
