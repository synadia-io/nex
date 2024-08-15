FROM golang AS nexbuilder
WORKDIR /nex
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o nexcli ./nex

FROM golang AS agentbuilder
WORKDIR /agent
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o nexagent ./agent/cmd/nex-agent

FROM debian:12-slim AS nex
RUN apt-get update \
    && apt-get install -y ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /
COPY --from=nexbuilder /nex/nexcli /usr/local/bin/nex
COPY --from=agentbuilder /agent/nexagent /usr/local/bin/nex-agent

FROM nex AS ci
COPY ./examples/nodeconfigs/simple.json nex.json
ENTRYPOINT ["nex", "node", "up", "--config", "nex.json"]

FROM nex
ENTRYPOINT ["nex"]
