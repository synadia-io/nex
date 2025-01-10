module github.com/synadia-labs/nex/client

go 1.23.4

replace github.com/synadia-labs/nex => ..

replace github.com/synadia-labs/nex/agents => ../agents

require (
	github.com/carlmjohnson/be v0.24.1
	github.com/nats-io/nats-server/v2 v2.10.25
	github.com/nats-io/nats.go v1.39.0
	github.com/nats-io/nkeys v0.4.9
	github.com/nats-io/nuid v1.0.1
	github.com/synadia-labs/nex v0.0.0-20250207230947-db8f5631d618
)

require (
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/nats-io/jwt/v2 v2.7.3 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.1 // indirect
	github.com/synadia-io/nexlet.go v0.0.0-20250206183151-4afda8a1a9b7 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	golang.org/x/time v0.10.0 // indirect
)
