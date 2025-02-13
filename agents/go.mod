module github.com/synadia-labs/nex/agents

go 1.24.0

replace github.com/synadia-labs/nex => ..

require (
	github.com/goombaio/namegenerator v0.0.0-20181006234301-989e774b106e
	github.com/nats-io/nats.go v1.39.0
	github.com/nats-io/nkeys v0.4.10
	github.com/synadia-io/nexlet.go v0.0.0-20250213161520-59ed74a1f231
	github.com/synadia-labs/nex v0.0.0-20250212004254-da283100238c
	golang.org/x/sys v0.30.0
)

require (
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.1 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/text v0.22.0 // indirect
)
