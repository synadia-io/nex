package actors

import (
	"time"

	"github.com/synadia-io/nex/api/nodecontrol/gen"
	api "github.com/synadia-io/nex/api/nodecontrol/gen"
	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

func startRequestToProto(request *api.StartWorkloadRequestJson) *actorproto.StartWorkload {
	return &actorproto.StartWorkload{
		Argv:        request.Argv,
		Description: request.Description,
		Environment: request.Environment,
		Essential:   request.Essential,
		Hash:        request.Hash,
		HostServiceConfig: &actorproto.HostServicesConfig{
			NatsUrl:      request.HostServiceConfig.NatsUrl,
			NatsUserSeed: request.HostServiceConfig.NatsUserSeed,
			NatsUserJwt:  request.HostServiceConfig.NatsUserJwt,
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
	return &actorproto.StopWorkload{
		NodeId:      request.NodeId,
		WorkloadId:  request.WorkloadId,
		WorkloadJwt: request.WorkloadJwt,
	}
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

func auctionResponseFromProto(response *actorproto.AuctionResponse) *api.AuctionResponseJson {
	convertedStatus := make(map[string]int)
	if response.Status != nil {
		for k, v := range response.Status {
			convertedStatus[k] = int(v)
		}
	}

	return &api.AuctionResponseJson{
		Nexus:      response.Nexus,
		NodeId:     response.NodeId,
		Status:     gen.AuctionResponseJsonStatus{Status: convertedStatus},
		Tags:       api.AuctionResponseJsonTags{Tags: response.Tags},
		TargetXkey: response.TargetXkey,
		Uptime:     time.Since(response.StartedAt.AsTime()).String(),
		Version:    response.Version,
	}
}

func lameDuckResponseFromProto(response *actorproto.LameDuckResponse) *api.LameduckResponseJson {
	return &api.LameduckResponseJson{
		Id:      response.Id,
		Success: response.Success,
	}
}

func pingResponseFromProto(response *actorproto.PingNodeResponse) *api.NodePingResponseJson {
	convertedStatus := make(map[string]int)
	if response.RunningAgents != nil {
		for k, v := range response.RunningAgents {
			convertedStatus[k] = int(v)
		}
	}
	return &api.NodePingResponseJson{
		Nexus:         response.Nexus,
		NodeId:        response.NodeId,
		RunningAgents: api.NodePingResponseJsonRunningAgents{Status: convertedStatus},
		Tags:          api.NodePingResponseJsonTags{Tags: response.Tags},
		TargetXkey:    response.TargetXkey,
		Uptime:        time.Since(response.StartedAt.AsTime()).String(),
		Version:       response.Version,
	}
}

func agentPingResponseFromProto(response *actorproto.PingAgentResponse) *api.AgentPingResponseJson {
	ret := new(api.AgentPingResponseJson)

	for _, w := range response.RunningWorkloads {
		ret.RunningWorkloads = append(ret.RunningWorkloads, api.WorkloadPingMachineSummary{
			Id:        w.Id,
			Name:      w.Name,
			Namespace: w.Namespace,
		})
	}

	ret.NodeId = response.NodeId
	ret.Tags = api.AgentPingResponseJsonTags{Tags: response.Tags}
	ret.TargetXkey = response.TargetXkey
	ret.Uptime = time.Since(response.StartedAt.AsTime()).String()
	ret.Version = response.Version
	return ret
}

func workloadPingResponseFromProto(response *actorproto.PingWorkloadResponse) *api.WorkloadPingResponseJson {
	return &api.WorkloadPingResponseJson{
		NodeId: response.NodeId,
		WorkloadSummary: &api.Workload{
			Id:           response.Workload.Id,
			Name:         response.Workload.Name,
			Runtime:      response.Workload.Runtime,
			StartTime:    response.Workload.StartedAt.String(),
			WorkloadType: response.Workload.WorkloadType,
		},
	}
}
