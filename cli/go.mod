module github.com/synadia-labs/nex/cli

go 1.24.0

replace github.com/synadia-labs/nex => ..

replace github.com/synadia-labs/nex/agents => ../agents

replace github.com/synadia-labs/nex/client => ../client

require (
	disorder.dev/shandler v0.0.0-20250324143720-5c3a7875bfd2
	github.com/alecthomas/kong v1.9.0
	github.com/carlmjohnson/be v0.24.1
	github.com/jedib0t/go-pretty/v6 v6.6.7
	github.com/nats-io/jsm.go v0.2.0
	github.com/nats-io/nats-server/v2 v2.11.0
	github.com/nats-io/nats.go v1.40.1
	github.com/nats-io/natscli v0.2.0
	github.com/nats-io/nkeys v0.4.10
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.1
	github.com/stretchr/testify v1.10.0
	github.com/synadia-labs/nex v0.0.0-20250326161951-0bc49a8a093e
	github.com/synadia-labs/nex/agents v0.0.0-20250326161951-0bc49a8a093e
	github.com/synadia-labs/nex/client v0.0.0-20250326161951-0bc49a8a093e
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/colorprofile v0.3.0 // indirect
	github.com/charmbracelet/lipgloss v1.1.0 // indirect
	github.com/charmbracelet/x/ansi v0.8.0 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.13 // indirect
	github.com/charmbracelet/x/term v0.2.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/expr-lang/expr v1.17.2 // indirect
	github.com/google/go-tpm v0.9.3 // indirect
	github.com/goombaio/namegenerator v0.0.0-20181006234301-989e774b106e // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/nats-io/jwt/v2 v2.7.3 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/synadia-io/nexlet.go v0.0.0-20250327030943-560b76a8cb04 // indirect
	github.com/synadia-io/orbit.go/natsext v0.1.1 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/net v0.37.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/term v0.30.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
