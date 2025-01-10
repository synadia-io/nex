module github.com/synadia-labs/nex/agents

go 1.23.4

replace github.com/synadia-labs/nex => ..

require (
	github.com/goombaio/namegenerator v0.0.0-20181006234301-989e774b106e
	github.com/nats-io/nats.go v1.39.0
	github.com/nats-io/nkeys v0.4.9
	github.com/synadia-io/nexlet.go v0.0.0-20250206183151-4afda8a1a9b7
	github.com/synadia-labs/nex v0.0.0-20250207230947-db8f5631d618
	golang.org/x/sys v0.30.0
)

require (
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.1 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/text v0.22.0 // indirect
)
