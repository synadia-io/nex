package credentials

import (
	"fmt"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/synadia-labs/nex/models"
)

var (
	AgentRegistrationClaims func(string, string) jwt.Permissions = func(id, nodeId string) jwt.Permissions {
		return jwt.Permissions{
			Pub: jwt.Permission{
				Allow: []string{fmt.Sprintf("$NEX.agent.%s.REGISTER.%s", id, nodeId)},
			},
			Sub: jwt.Permission{
				Allow: []string{nats.InboxPrefix + ">"},
			},
			Resp: &jwt.ResponsePermission{
				MaxMsgs: 0,
			},
		}
	}
	AgentClaims func(string, string) jwt.Permissions = func(id, nodeId string) jwt.Permissions {
		return jwt.Permissions{
			Pub: jwt.Permission{
				Allow: []string{
					fmt.Sprintf("%s.%s.>", models.AgentAPIPrefix, id),
					fmt.Sprintf("%s.%s.*", models.EventAPIPrefix, id),
					fmt.Sprintf("%s.*.*.stdout", models.LogAPIPrefix),  // workload logs
					fmt.Sprintf("%s.*.*.stderr", models.LogAPIPrefix),  // workload logs
					fmt.Sprintf("%s.*.*.metrics", models.LogAPIPrefix), // workload metrics
					"update_caddy",         // temporary for caddy
					nats.InboxPrefix + ">", // responses
				},
			},
			Sub: jwt.Permission{
				Allow: []string{
					fmt.Sprintf("%s.%s.EVENT", models.AgentAPIPrefix, id), // TODO: this should be EVENTS
					fmt.Sprintf("%s.%s.%s.STARTWORKLOAD.*", models.AgentAPIPrefix, nodeId, id),
					fmt.Sprintf("%s.%s.%s.PING", models.AgentAPIPrefix, nodeId, id),
					fmt.Sprintf("%s.%s.PING", models.AgentAPIPrefix, nodeId),
					fmt.Sprintf("%s.%s.STOPWORKLOAD.*", models.AgentAPIPrefix, nodeId),
					fmt.Sprintf("%s.%s.GETWORKLOAD.*", models.AgentAPIPrefix, nodeId),
					fmt.Sprintf("%s.%s.QUERYWORKLOADS", models.AgentAPIPrefix, nodeId),
					fmt.Sprintf("%s.%s.SETLAMEDUCK", models.AgentAPIPrefix, nodeId),
					fmt.Sprintf("%s.PINGWORKLOAD.*", models.AgentAPIPrefix),
					fmt.Sprintf("%s.*.*.TRIGGER", models.WorkloadAPIPrefix), // BUG: This should use the workload credentials
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
				// $NEX.workload.<namespace>.<id>
				Allow: []string{
					fmt.Sprintf("%s.%s.%s.metrics", models.LogAPIPrefix, namespace, workloadId), // workload metrics
					fmt.Sprintf("%s.%s.>", models.WorkloadAPIPrefix, namespace),
					nats.InboxPrefix + ">", // responses
				},
			},
			Sub: jwt.Permission{
				Allow: []string{
					fmt.Sprintf("%s.%s.>", models.LogAPIPrefix, namespace),
					fmt.Sprintf("%s.%s.>", models.WorkloadAPIPrefix, namespace),
					nats.InboxPrefix + ">", // responses
				},
			},
			Resp: &jwt.ResponsePermission{
				MaxMsgs: 1,
			},
		}
	}
)
