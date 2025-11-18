# syntax=docker/dockerfile:1.4
FROM golang:latest AS builder

ENV GOWORK=off
ENV CGO_ENABLED=0

# Git is required for fetching modules
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
# Copy the sdk/go directory to satisfy the replace directive
COPY sdk/go/ ./sdk/go/
COPY client/ ./client
RUN go mod download

COPY . .
WORKDIR /app/cmd/nex
RUN go build -o /nex

FROM scratch
COPY --from=builder /nex /nex
ENTRYPOINT ["/nex"]
CMD ["--help"]
