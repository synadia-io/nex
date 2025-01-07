package actors

import (
	"time"

	api "github.com/synadia-io/nex/api/go"
	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

func startRequestToProto(request *api.StartWorkloadRequest) *actorproto.StartWorkload {
	return &actorproto.StartWorkload{
		Argv:        request.Argv,
		Description: request.Description,
		Environment: &actorproto.EncEnvironment{
			EncryptedBy:        request.EncEnvironment.EncryptedBy,
			Base64EncryptedEnv: request.EncEnvironment.Base64EncryptedEnv,
		},
		Hash: request.Hash,
		HostServiceConfig: &actorproto.HostServicesConfig{
			NatsUrl:      request.HostServiceConfig.NatsUrl,
			NatsUserSeed: request.HostServiceConfig.NatsUserSeed,
			NatsUserJwt:  request.HostServiceConfig.NatsUserJwt,
		},
		Jsdomain:        request.Jsdomain,
		Namespace:       request.Namespace,
		RetryCount:      int32(request.RetryCount),
		TriggerSubject:  request.TriggerSubject,
		Uri:             request.Uri,
		WorkloadName:    request.WorkloadName,
		WorkloadType:    request.WorkloadType,
		WorkloadRuntype: request.WorkloadRuntype,
	}
}

func startRequestFromProto(request *actorproto.StartWorkload) *api.StartWorkloadRequest {
	return &api.StartWorkloadRequest{
		Argv:        request.Argv,
		Description: request.Description,
		EncEnvironment: api.EncEnv{
			Base64EncryptedEnv: request.Environment.Base64EncryptedEnv,
			EncryptedBy:        request.Environment.EncryptedBy,
		},
		Hash: request.Hash,
		HostServiceConfig: api.HostService{
			NatsUrl:      request.HostServiceConfig.NatsUrl,
			NatsUserJwt:  request.HostServiceConfig.NatsUserJwt,
			NatsUserSeed: request.HostServiceConfig.NatsUserSeed,
		},
		Jsdomain:        request.Jsdomain,
		Namespace:       request.Namespace,
		RetryCount:      int(request.RetryCount),
		TriggerSubject:  request.TriggerSubject,
		Uri:             request.Uri,
		WorkloadName:    request.WorkloadName,
		WorkloadType:    request.WorkloadType,
		WorkloadRuntype: request.WorkloadRuntype,
	}
}

func startResponseFromProto(response *actorproto.WorkloadStarted) *api.StartWorkloadResponse {
	return &api.StartWorkloadResponse{
		Id:      response.Id,
		Issuer:  response.Issuer,
		Name:    response.Name,
		Started: response.Started,
	}
}

func stopResponseFromProto(response *actorproto.WorkloadStopped) *api.StopWorkloadResponse {
	return &api.StopWorkloadResponse{
		Id:      response.Id,
		Issuer:  response.Issuer,
		Stopped: response.Stopped,
	}
}

func infoResponseFromProto(response *actorproto.NodeInfo) *api.NodeInfoResponse {
	ret := new(api.NodeInfoResponse)
	ret.NodeId = response.Id
	ret.Tags = api.NodeInfoResponseTags{Tags: response.Tags}
	ret.TargetXkey = response.TargetXkey
	ret.Uptime = response.Uptime
	ret.Version = response.Version

	for _, workload := range response.Workloads {
		ret.WorkloadSummaries = append(ret.WorkloadSummaries, api.WorkloadSummary{
			Id:              workload.Id,
			Name:            workload.Name,
			Runtime:         workload.Runtime,
			StartTime:       workload.StartedAt.AsTime().Format(time.DateTime),
			WorkloadType:    workload.WorkloadType,
			WorkloadRuntype: workload.WorkloadRuntype,
			WorkloadState:   workload.State,
		})
	}
	return ret
}

func auctionResponseFromProto(response *actorproto.AuctionResponse) *api.AuctionResponse {
	convertedStatus := make(map[string]int)
	if response.Status != nil {
		for k, v := range response.Status {
			convertedStatus[k] = int(v)
		}
	}

	return &api.AuctionResponse{
		BidderId:   response.BidderId,
		Status:     api.AuctionResponseStatus{Status: convertedStatus},
		Tags:       api.AuctionResponseTags{Tags: response.Tags},
		TargetXkey: response.TargetXkey,
		Uptime:     time.Since(response.StartedAt.AsTime()).String(),
		Version:    response.Version,
	}
}

func pingResponseFromProto(response *actorproto.PingNodeResponse) *api.NodePingResponse {
	convertedStatus := make(map[string]int)
	if response.RunningAgents != nil {
		for k, v := range response.RunningAgents {
			convertedStatus[k] = int(v)
		}
	}
	return &api.NodePingResponse{
		NodeId:        response.NodeId,
		RunningAgents: api.NodePingResponseRunningAgents{Status: convertedStatus},
		Tags:          api.NodePingResponseTags{Tags: response.Tags},
		TargetXkey:    response.TargetXkey,
		Uptime:        time.Since(response.StartedAt.AsTime()).String(),
		Version:       response.Version,
	}
}

func workloadPingResponseFromProto(response *actorproto.PingWorkloadResponse) *api.WorkloadPingResponse {
	return &api.WorkloadPingResponse{
		WorkloadSummary: &api.Workload{
			Id:              response.Workload.Id,
			Name:            response.Workload.Name,
			Runtime:         response.Workload.Runtime,
			StartTime:       response.Workload.StartedAt.AsTime().Format(time.DateTime),
			WorkloadType:    response.Workload.WorkloadType,
			WorkloadRuntype: response.Workload.WorkloadRuntype,
			WorkloadState:   response.Workload.State,
		},
	}
}
