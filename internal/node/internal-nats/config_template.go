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
 * agentint.> - where the first token after agentint is the account ID from the workload
 */

const (
	configTemplate = `
jetstream: true

accounts: {
	nexhost: {
		jetstream: true		
		users: [
			{user: nex, password: pass}
			{nkey: "{{ .NexHostUserPublic }}"}
		]
		exports: [
			{
				service: agentint.>
			}
		],
		imports: [
			{{ range .Credentials }}
			{
				stream: {subject: agentint.>, account: {{ .ID }}}
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
		imports: [
			{service: {account: nexhost, subject: agentint.{{ .ID }}.>}, to: agentint.>}
		]
		exports: [
			{stream: agentint.>, accounts: [nexhost]}
			{stream: agentevt.>, accounts: [nexhost]}
		]
	},
	{{ end }}	
}
no_sys_acc: true
`
)

func GenerateTemplate(log *slog.Logger, config internalServerData) ([]byte, error) {
	var wr bytes.Buffer

	t := template.Must(template.New("natsconfig").Parse(configTemplate))
	err := t.Execute(&wr, config)
	if err != nil {
		return nil, err
	}

	log.Debug("generated NATS config", slog.String("config", string(wr.Bytes())))
	return wr.Bytes(), nil
}
