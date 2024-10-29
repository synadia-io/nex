package actors

import (
	"github.com/synadia-io/nex/api/nodecontrol/gen"
	api "github.com/synadia-io/nex/api/nodecontrol/gen"
	actorproto "github.com/synadia-io/nex/node/actors/pb"
)

func startRequestToProto(request *api.StartWorkloadRequestJson) *actorproto.StartWorkload {
	return &actorproto.StartWorkload{
		Argv:        request.Argv,
		Description: request.Description,
		Environment: request.Environment,
		Essential:   request.Essential,
		Hash:        request.Hash,
		HostServiceConfig: &actorproto.HostServicesConfig{
			NatsUrl: func() string {
				if request.HostServiceConfig.NatsUrl == nil {
					return ""
				} else {
					return *request.HostServiceConfig.NatsUrl
				}
			}(),
			NatsUserSeed: func() string {
				if request.HostServiceConfig.NatsUserSeed == nil {
					return ""
				} else {
					return *request.HostServiceConfig.NatsUserSeed
				}
			}(),
			NatsUserJwt: func() string {
				if request.HostServiceConfig.NatsUserJwt == nil {
					return ""
				} else {
					return *request.HostServiceConfig.NatsUserJwt
				}
			}(),
		},
		Jsdomain:        request.Jsdomain,
		RetriedAt:       request.RetriedAt,
		RetryCount:      int32(request.RetryCount),
		TriggerSubjects: request.TriggerSubjects,
		Uri:             request.Uri,
		WorkloadJwt:     request.WorkloadJwt,
		WorkloadName:    request.WorkloadName,
		WorkloadType:    request.WorkloadType,
	}
}

func startResponseFromProto(response *actorproto.WorkloadStarted) *api.StartWorkloadResponseJson {
	return &api.StartWorkloadResponseJson{
		Id:      response.Id,
		Issuer:  response.Issuer,
		Name:    response.Name,
		Started: response.Started,
	}
}

func stopRequestToProto(request *api.StopWorkloadRequestJson) *actorproto.StopWorkload {
	return &actorproto.StopWorkload{}
}

func stopResponseFromProto(response *actorproto.WorkloadStopped) *api.StopWorkloadResponseJson {
	return &api.StopWorkloadResponseJson{
		Id:      response.Id,
		Issuer:  response.Issuer,
		Stopped: response.Stopped,
	}
}

func infoResponseFromProto(response *actorproto.NodeInfo) *api.NodeInfoResponseJson {
	ret := new(api.NodeInfoResponseJson)
	ret.Nexus = response.Nexus
	ret.NodeId = response.Id
	ret.Tags = gen.NodeInfoResponseJsonTags{Tags: response.Tags}
	ret.TargetXkey = response.TargetXkey
	ret.Uptime = response.Uptime
	ret.Version = response.Version

	for _, workload := range response.Workloads {
		ret.WorkloadSummaries = append(ret.WorkloadSummaries, api.WorkloadSummary{
			Id:           workload.Id,
			Name:         workload.Name,
			Runtime:      workload.Runtime,
			StartTime:    workload.StartedAt.String(),
			WorkloadType: workload.WorkloadType,
		})
	}
	return ret
}
