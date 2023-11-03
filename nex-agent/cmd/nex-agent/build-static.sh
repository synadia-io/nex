#!/bin/bash

set -xe

export GIN_MODE=release
go build -tags netgo -ldflags '-extldflags "-static"'