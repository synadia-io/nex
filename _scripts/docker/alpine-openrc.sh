#!/sbin/openrc-run

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
}
