#!/bin/bash

apk add --no-cache iptables
apk add --no-cache iptables-legacy
apk add --no-cache sudo

echo '%nex ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/nex
