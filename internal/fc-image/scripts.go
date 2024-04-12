package rootfs

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

	systemd_service = `[Unit]
Description=Nex Agent
After=network.target

[Service]
User=nex
Group=nex
ExecStart=/usr/local/bin/agent
KillSignal=SIGINT
StandardOutput=file:/home/user/nex.log
StandardError=file:/home/user/err.log
Type=simple
Restart=always


[Install]
WantedBy=default.target
RequiredBy=network.target
`

	setup_alpine = `#!/bin/sh

set -xe

chmod +x /usr/local/bin/agent
chmod +x /etc/init.d/agent

apk add --no-cache openrc
apk add --no-cache util-linux

ln -s agetty /etc/init.d/agetty.ttyS0
echo ttyS0 >/etc/securetty
rc-update add agetty.ttyS0 default

echo "root:root" | chpasswd

addgroup -g 1000 -S nex && adduser -u 1000 -S nex -G nex

rc-update add devfs boot
rc-update add procfs boot
rc-update add sysfs boot

# This is our script that runs nex-agent
rc-update add agent boot

for d in bin etc lib root sbin usr; do tar c "/$d" | tar x -C /tmp/rootfs; done
for dir in dev proc run sys var tmp; do mkdir /tmp/rootfs/${dir}; done

chmod 1777 /tmp/rootfs/tmp
mkdir -p /tmp/rootfs/home/nex/
chown 1000:1000 /tmp/rootfs/home/nex/`

	setup_debian = `#!/bin/sh
`

	copy_fs = `#!/bin/sh
set -xe

for d in bin etc lib root sbin usr; do tar c "/$d" | tar x -C /tmp/rootfs; done
for dir in dev proc run sys var tmp; do mkdir /tmp/rootfs/${dir}; done

chmod 1777 /tmp/rootfs/tmp
mkdir -p /tmp/rootfs/home/nex/
chown 1000:1000 /tmp/rootfs/home/nex/`
)
