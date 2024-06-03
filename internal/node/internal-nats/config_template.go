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
			{nkey: "{{ .NexHostUserPublic }}"}
		]
		exports: [
			{
				service: agentint.>
			}
		],
		imports: [
			{{ range .Users }}
			{
				stream: {subject: agentevt.>, account: {{ .WorkloadID }}}, prefix: {{ .WorkloadID }}
			},
			{{ end }}
		]	
	},
	{{ range .Users }}
	{{ .WorkloadID }}: {
		jetstream: true
		users: [
			{nkey: "{{ .NkeyPublic }}"}
		]
		imports: [
			{service: {account: nexhost, subject: agentint.{{ .WorkloadID }}.>}, to: agentint.>}
		]
		exports: [
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
