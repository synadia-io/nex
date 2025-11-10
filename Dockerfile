# syntax=docker/dockerfile:1.4
FROM golang:latest AS builder

ENV GOWORK=off
ARG GOPRIVATE
ENV GOPRIVATE=$GOPRIVATE
ENV CGO_ENABLED=0

RUN apt-get update && apt-get install -y --no-install-recommends git openssh-client
RUN git config --global url."git@github.com:".insteadOf "https://github.com/"

RUN --mount=type=ssh mkdir -p /root/.ssh && ssh-keyscan github.com >> /root/.ssh/known_hosts

WORKDIR /app
COPY go.mod go.sum ./
# Copy the sdk/go directory to satisfy the replace directive
COPY sdk/go/ ./sdk/go/
COPY client/ ./client
RUN --mount=type=ssh go mod download

COPY . .
WORKDIR /app/cli
RUN --mount=type=ssh go build -o /nex

FROM scratch
COPY --from=builder /nex /nex
ENTRYPOINT ["/nex"]
CMD ["--help"]
