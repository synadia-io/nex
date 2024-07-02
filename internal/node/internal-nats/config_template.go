package internalnats

import (
	"bytes"
	"log/slog"
	"text/template"
)

type internalServerData struct {
	Credentials       map[string]*credentials
	NexHostUserPublic string
	NexHostUserSeed   string
}

type credentials struct {
	ID         string
	NkeySeed   string
	NkeyPublic string
}

/*
 * In the below template, the nexhost (the account used by the host) will
 * be able to import the following:
 * *.agentevt.> - agent events streamed from workloads
 * hostint.> - where the first token after agentint is the account ID from the workload
 */

const (
	configTemplate = `
jetstream: true
accounts: {
	nexhost: {
		jetstream: true		
		users: [
			{nkey: "{{ .NexHostUserPublic }}"}
		]
		exports: [
			{
				service: hostint.>
			}
		],
		imports: [
			{{ range .Credentials }}
			{
				service: {subject: agentint.{{ .ID }}.>, account: {{ .ID }}}
			},
			{
				stream: {subject: agentevt.>, account: {{ .ID }}}, prefix: {{ .ID }}
			},
			{{ end }}
		]
	},
	{{ range .Credentials }}
	{{ .ID }}: {
		jetstream: true
		users: [
			{nkey: "{{ .NkeyPublic }}"}
		]
		exports: [
			{
				service: agentint.{{ .ID }}.>, accounts: [nexhost]
			}
			{
				stream: agentevt.>, accounts: [nexhost]
			}
		]
		imports: [
			{
				service: {account: nexhost, subject: hostint.{{ .ID }}.>}
			}
		]

	},
	{{ end }}
}
no_sys_acc: true
debug: true
trace: false
`
)

func GenerateTemplate(log *slog.Logger, config internalServerData) ([]byte, error) {
	var wr bytes.Buffer

	t := template.Must(template.New("natsconfig").Parse(configTemplate))
	err := t.Execute(&wr, config)
	if err != nil {
		return nil, err
	}

	// -8 is equivalent to TRACE-ish level
	log.Debug("generated NATS config", slog.String("config", wr.String()))
	return wr.Bytes(), nil
}
