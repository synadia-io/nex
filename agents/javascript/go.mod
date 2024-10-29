module github.com/synadia-io/nex/agents/javascript

go 1.23.2

replace github.com/synadia-io/nex/agents/common => ../common

require github.com/synadia-io/nex/agents/common v0.0.0-00010101000000-000000000000

require (
	github.com/klauspost/compress v1.17.2 // indirect
	github.com/nats-io/nats.go v1.37.0 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.18.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
)
