module github.com/synadia-labs/nex/client

go 1.24.0

replace github.com/synadia-labs/nex => ..

replace github.com/synadia-labs/nex/agents => ../agents

require (
	github.com/carlmjohnson/be v0.24.1
	github.com/nats-io/nats.go v1.39.1
	github.com/nats-io/nkeys v0.4.10
	github.com/nats-io/nuid v1.0.1
	github.com/synadia-labs/nex v0.0.0-20250221042458-45e17023d430
)

require (
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/nats-io/jwt/v2 v2.7.3 // indirect
	github.com/nats-io/nats-server/v2 v2.10.25 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.1 // indirect
	github.com/synadia-io/nexlet.go v0.0.0-20250224193051-85b1a77a3bf9 // indirect
	golang.org/x/crypto v0.35.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	golang.org/x/time v0.10.0 // indirect
)
