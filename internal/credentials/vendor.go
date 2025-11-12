package credentials

import (
	"fmt"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/models"
)

var (
	AgentRegistrationClaims func(string, string) jwt.Permissions = func(id, nodeId string) jwt.Permissions {
		return jwt.Permissions{
			Pub: jwt.Permission{
				Allow: []string{
					fmt.Sprintf("%s.REGISTER.%s", models.AgentAPIPrefix(nodeId), id),
				},
			},
			Sub: jwt.Permission{
				Allow: []string{
					nats.InboxPrefix + ">",
				},
			},
			Resp: &jwt.ResponsePermission{
				MaxMsgs: 0,
			},
		}
	}
	AgentClaims func(string, string, string) jwt.Permissions = func(id, nodeId, nexus string) jwt.Permissions {
		return jwt.Permissions{
			Pub: jwt.Permission{
				Allow: []string{
					fmt.Sprintf("%s.HEARTBEAT.%s", models.AgentAPIPrefix(nodeId), id),
					fmt.Sprintf("%s.*", models.EventAPIPrefix(id)),
					fmt.Sprintf("%s.*.stdout", models.LogAPIPrefix(id)), //
					fmt.Sprintf("%s.*.stderr", models.LogAPIPrefix(id)), // workload logs
					// 		"update_caddy",         // temporary for caddy
					nats.InboxPrefix + ">", // responses
				},
			},
			Sub: jwt.Permission{
				Allow: []string{
					fmt.Sprintf("%s.%s.STARTWORKLOAD.*", models.AgentAPIPrefix(nodeId), id),
					fmt.Sprintf("%s.%s.PING", models.AgentAPIPrefix(nodeId), id),
					fmt.Sprintf("%s.STOPWORKLOAD.*", models.AgentAPIPrefix(nodeId)),
					fmt.Sprintf("%s.GETWORKLOAD.*", models.AgentAPIPrefix(nodeId)),
					fmt.Sprintf("%s.QUERYWORKLOADS", models.AgentAPIPrefix(nodeId)),
					fmt.Sprintf("%s.PING", models.AgentAPIPrefix(nodeId)),
					fmt.Sprintf("%s.SETLAMEDUCK", models.AgentAPIPrefix(nodeId)),
					fmt.Sprintf("%s.PINGWORKLOAD.*", models.AgentAPIPrefix(nexus)),
					"$SRV.>",               // for micro
					nats.InboxPrefix + ">", // responses
				},
			},
			Resp: &jwt.ResponsePermission{
				MaxMsgs: 1,
			},
		}
	}
	WorkloadClaims func(string, string) jwt.Permissions = func(namespace, workloadId string) jwt.Permissions {
		return jwt.Permissions{
			Pub: jwt.Permission{
				Allow: []string{
					nats.InboxPrefix + ">", // responses
				},
			},
			Sub: jwt.Permission{
				Allow: []string{
					fmt.Sprintf("%s.>", models.LogAPIPrefix(namespace)),
					nats.InboxPrefix + ">", // responses
				},
			},
			Resp: &jwt.ResponsePermission{
				MaxMsgs: 1,
			},
		}
	}
)
