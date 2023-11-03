#!/sbin/openrc-run

name=$RC_SVCNAME
description="Nex Agent"
supervisor="supervise-daemon"
command="/usr/local/bin/agent"
pidfile="/run/agent.pid"
command_user="nex:nex"

depend() {
	after net
}