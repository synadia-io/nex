module github.com/synadia-labs/nex/cli

go 1.24.0

replace github.com/synadia-labs/nex => ..

replace github.com/synadia-labs/nex/agents => ../agents

replace github.com/synadia-labs/nex/client => ../client

require (
	disorder.dev/shandler v0.0.0-20250212150133-c29daa0276fc
	github.com/alecthomas/kong v1.8.0
	github.com/carlmjohnson/be v0.24.1
	github.com/jedib0t/go-pretty/v6 v6.6.6
	github.com/nats-io/jsm.go v0.1.2
	github.com/nats-io/nats-server/v2 v2.10.25
	github.com/nats-io/nats.go v1.39.0
	github.com/nats-io/natscli v0.1.6
	github.com/nats-io/nkeys v0.4.10
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.1
	github.com/stretchr/testify v1.10.0
	github.com/synadia-labs/nex v0.0.0-20250212004254-da283100238c
	github.com/synadia-labs/nex/agents v0.0.0-20250212004254-da283100238c
	github.com/synadia-labs/nex/client v0.0.0-20250212004254-da283100238c
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/lipgloss v1.0.0 // indirect
	github.com/charmbracelet/x/ansi v0.8.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/goombaio/namegenerator v0.0.0-20181006234301-989e774b106e // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/muesli/termenv v0.15.2 // indirect
	github.com/nats-io/jwt/v2 v2.7.3 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/synadia-io/nexlet.go v0.0.0-20250213161520-59ed74a1f231 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/term v0.29.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	golang.org/x/time v0.10.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
