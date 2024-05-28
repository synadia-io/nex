package internalnats

import (
	"bytes"
	"log/slog"
	"text/template"
)

type internalServerData struct {
	Users             []userData
	NexHostUserPublic string
	NexHostUserSeed   string
}

type userData struct {
	WorkloadID string
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
			{{range .Users }}
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

func GenerateFile(log *slog.Logger, config internalServerData) ([]byte, error) {
	t := template.Must(template.New("natsconfig").Parse(configTemplate))

	var wr bytes.Buffer
	err := t.Execute(&wr, config)
	if err != nil {
		return nil, err
	}
	return wr.Bytes(), nil
}

/*
exports: [
			{
				service: agentint.>
			}
		],
		imports: [
			{{range .Users }}
			{
				stream: {subject: agentevt.>, account: {{ .AccountPublicKey }}}, prefix: {{ .AccountPublicKey }} }
			},
			{{ end }}
		]
	},
	{{ range .Users }}
	{{ .AccountPublicKey }}: {
		users: [
			{nkey: {{ .NkeyPublic }}}
		]
		imports: [
			{service: {account: nexhost, subject: agentint.{{ .AccountPublicKey }}.>}, to: agentint.>}
		]
		exports: [
			{stream: agentevt.>, accounts: [nexhost]}
		]
	},
	{{ end }}
*/

/*

listen: 0.0.0.0:4229
accounts: {
    nexhost: {
        users: [
            {nkey: UCNGL4W5QX66CFX6A6DCBVDH5VOHMI7B2UZZU7TXAUQQSI2JPHULCKBR}
        ],
        exports: [
            {service: agentint.>}
        ]
        imports: [
            {stream: {subject: agentevt.>, account: PKIbeHP9gWD}, prefix: PKIbeHP9gWD}
        ]
    }
    PKIbeHP9gWD: {
        users: [
            {nkey: UDPGQVFIWZ7Q5UH4I5E6DBCZULQS6VTVBG6CYBD7JV3G3N2GMQOMNAUH}
        ]
        imports: [
            {service: {account: nexhost, subject: agentint.PKIbeHP9gWD.>}, to: agentint.>}
        ]
        exports: [
            {stream: agentevt.>, accounts: [nexhost]}
        ]
    }
}
*/
