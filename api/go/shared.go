// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package api

import "encoding/json"
import "fmt"

type AgentPingResponse struct {
	// The unique identifier of the node on which the agent is running
	NodeId string `json:"node_id"`

	// RunningWorkloads corresponds to the JSON schema field "running_workloads".
	RunningWorkloads []WorkloadPingMachineSummary `json:"running_workloads"`

	// Tags corresponds to the JSON schema field "tags".
	Tags AgentPingResponseTags `json:"tags"`

	// The target agents xkey
	TargetXkey string `json:"target_xkey"`

	// The uptime of the node
	Uptime string `json:"uptime"`

	// The target agents version
	Version string `json:"version"`
}

type AgentPingResponseTags struct {
	// Tags corresponds to the JSON schema field "tags".
	Tags AgentPingResponseTagsTags `json:"tags,omitempty"`
}

type AgentPingResponseTagsTags map[string]string

// UnmarshalJSON implements json.Unmarshaler.
func (j *AgentPingResponse) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["node_id"]; raw != nil && !ok {
		return fmt.Errorf("field node_id in AgentPingResponse: required")
	}
	if _, ok := raw["running_workloads"]; raw != nil && !ok {
		return fmt.Errorf("field running_workloads in AgentPingResponse: required")
	}
	if _, ok := raw["tags"]; raw != nil && !ok {
		return fmt.Errorf("field tags in AgentPingResponse: required")
	}
	if _, ok := raw["target_xkey"]; raw != nil && !ok {
		return fmt.Errorf("field target_xkey in AgentPingResponse: required")
	}
	if _, ok := raw["uptime"]; raw != nil && !ok {
		return fmt.Errorf("field uptime in AgentPingResponse: required")
	}
	if _, ok := raw["version"]; raw != nil && !ok {
		return fmt.Errorf("field version in AgentPingResponse: required")
	}
	type Plain AgentPingResponse
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = AgentPingResponse(plain)
	return nil
}

type EncEnv struct {
	// Base64EncryptedEnv corresponds to the JSON schema field "base64_encrypted_env".
	Base64EncryptedEnv string `json:"base64_encrypted_env"`

	// EncryptedBy corresponds to the JSON schema field "encrypted_by".
	EncryptedBy string `json:"encrypted_by"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *EncEnv) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["base64_encrypted_env"]; raw != nil && !ok {
		return fmt.Errorf("field base64_encrypted_env in EncEnv: required")
	}
	if _, ok := raw["encrypted_by"]; raw != nil && !ok {
		return fmt.Errorf("field encrypted_by in EncEnv: required")
	}
	type Plain EncEnv
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = EncEnv(plain)
	return nil
}

type HostService struct {
	// NatsUrl corresponds to the JSON schema field "nats_url".
	NatsUrl string `json:"nats_url"`

	// NatsUserJwt corresponds to the JSON schema field "nats_user_jwt".
	NatsUserJwt string `json:"nats_user_jwt"`

	// NatsUserSeed corresponds to the JSON schema field "nats_user_seed".
	NatsUserSeed string `json:"nats_user_seed"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *HostService) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["nats_url"]; raw != nil && !ok {
		return fmt.Errorf("field nats_url in HostService: required")
	}
	if _, ok := raw["nats_user_jwt"]; raw != nil && !ok {
		return fmt.Errorf("field nats_user_jwt in HostService: required")
	}
	if _, ok := raw["nats_user_seed"]; raw != nil && !ok {
		return fmt.Errorf("field nats_user_seed in HostService: required")
	}
	type Plain HostService
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = HostService(plain)
	return nil
}

type LameduckRequest struct {
	// Time delay before lameduck mode is set
	Delay string `json:"delay"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *LameduckRequest) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["delay"]; raw != nil && !ok {
		return fmt.Errorf("field delay in LameduckRequest: required")
	}
	type Plain LameduckRequest
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = LameduckRequest(plain)
	return nil
}

type LameduckResponse struct {
	// Indicates lameduck mode successfully set
	Success bool `json:"success"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *LameduckResponse) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["success"]; raw != nil && !ok {
		return fmt.Errorf("field success in LameduckResponse: required")
	}
	type Plain LameduckResponse
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = LameduckResponse(plain)
	return nil
}

type StartWorkloadRequest struct {
	// Arguments to be passed to the binary
	Argv []string `json:"argv"`

	// A description of the workload
	Description string `json:"description"`

	// The base64-encoded byte array of the encrypted environment with public key of
	// encryptor
	EncEnvironment EncEnv `json:"enc_environment"`

	// The hash of the workload
	Hash string `json:"hash"`

	// The NATS configuration for the host services
	HostServiceConfig HostService `json:"host_service_config"`

	// The NATS JSDomain for the workload
	Jsdomain string `json:"jsdomain"`

	// The namespace of the workload
	Namespace string `json:"namespace"`

	// The number of times the workload has been retried
	RetryCount int `json:"retry_count"`

	// The public key of the sender
	SenderPublicKey string `json:"sender_public_key"`

	// The xkey of the target node
	TargetPubXkey string `json:"target_pub_xkey"`

	// The subject that triggers the workload
	TriggerSubject string `json:"trigger_subject"`

	// The URI of the workload
	Uri string `json:"uri"`

	// The JWT for the workload
	WorkloadJwt string `json:"workload_jwt"`

	// The name of the workload
	WorkloadName string `json:"workload_name"`

	// The runtype of the workload
	WorkloadRuntype string `json:"workload_runtype"`

	// The type of the workload
	WorkloadType string `json:"workload_type"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *StartWorkloadRequest) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["argv"]; raw != nil && !ok {
		return fmt.Errorf("field argv in StartWorkloadRequest: required")
	}
	if _, ok := raw["description"]; raw != nil && !ok {
		return fmt.Errorf("field description in StartWorkloadRequest: required")
	}
	if _, ok := raw["enc_environment"]; raw != nil && !ok {
		return fmt.Errorf("field enc_environment in StartWorkloadRequest: required")
	}
	if _, ok := raw["hash"]; raw != nil && !ok {
		return fmt.Errorf("field hash in StartWorkloadRequest: required")
	}
	if _, ok := raw["host_service_config"]; raw != nil && !ok {
		return fmt.Errorf("field host_service_config in StartWorkloadRequest: required")
	}
	if _, ok := raw["jsdomain"]; raw != nil && !ok {
		return fmt.Errorf("field jsdomain in StartWorkloadRequest: required")
	}
	if _, ok := raw["namespace"]; raw != nil && !ok {
		return fmt.Errorf("field namespace in StartWorkloadRequest: required")
	}
	if _, ok := raw["retry_count"]; raw != nil && !ok {
		return fmt.Errorf("field retry_count in StartWorkloadRequest: required")
	}
	if _, ok := raw["sender_public_key"]; raw != nil && !ok {
		return fmt.Errorf("field sender_public_key in StartWorkloadRequest: required")
	}
	if _, ok := raw["target_pub_xkey"]; raw != nil && !ok {
		return fmt.Errorf("field target_pub_xkey in StartWorkloadRequest: required")
	}
	if _, ok := raw["trigger_subject"]; raw != nil && !ok {
		return fmt.Errorf("field trigger_subject in StartWorkloadRequest: required")
	}
	if _, ok := raw["uri"]; raw != nil && !ok {
		return fmt.Errorf("field uri in StartWorkloadRequest: required")
	}
	if _, ok := raw["workload_jwt"]; raw != nil && !ok {
		return fmt.Errorf("field workload_jwt in StartWorkloadRequest: required")
	}
	if _, ok := raw["workload_name"]; raw != nil && !ok {
		return fmt.Errorf("field workload_name in StartWorkloadRequest: required")
	}
	if _, ok := raw["workload_runtype"]; raw != nil && !ok {
		return fmt.Errorf("field workload_runtype in StartWorkloadRequest: required")
	}
	if _, ok := raw["workload_type"]; raw != nil && !ok {
		return fmt.Errorf("field workload_type in StartWorkloadRequest: required")
	}
	type Plain StartWorkloadRequest
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = StartWorkloadRequest(plain)
	return nil
}

type StartWorkloadResponse struct {
	// Id corresponds to the JSON schema field "id".
	Id string `json:"id"`

	// Issuer corresponds to the JSON schema field "issuer".
	Issuer string `json:"issuer"`

	// Message corresponds to the JSON schema field "message".
	Message string `json:"message"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name"`

	// Started corresponds to the JSON schema field "started".
	Started bool `json:"started"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *StartWorkloadResponse) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["id"]; raw != nil && !ok {
		return fmt.Errorf("field id in StartWorkloadResponse: required")
	}
	if _, ok := raw["issuer"]; raw != nil && !ok {
		return fmt.Errorf("field issuer in StartWorkloadResponse: required")
	}
	if _, ok := raw["message"]; raw != nil && !ok {
		return fmt.Errorf("field message in StartWorkloadResponse: required")
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in StartWorkloadResponse: required")
	}
	if _, ok := raw["started"]; raw != nil && !ok {
		return fmt.Errorf("field started in StartWorkloadResponse: required")
	}
	type Plain StartWorkloadResponse
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = StartWorkloadResponse(plain)
	return nil
}

type StopWorkloadRequest struct {
	// Namespace corresponds to the JSON schema field "namespace".
	Namespace string `json:"namespace"`

	// WorkloadId corresponds to the JSON schema field "workload_id".
	WorkloadId string `json:"workload_id"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *StopWorkloadRequest) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["namespace"]; raw != nil && !ok {
		return fmt.Errorf("field namespace in StopWorkloadRequest: required")
	}
	if _, ok := raw["workload_id"]; raw != nil && !ok {
		return fmt.Errorf("field workload_id in StopWorkloadRequest: required")
	}
	type Plain StopWorkloadRequest
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = StopWorkloadRequest(plain)
	return nil
}

type StopWorkloadResponse struct {
	// Id corresponds to the JSON schema field "id".
	Id string `json:"id"`

	// Issuer corresponds to the JSON schema field "issuer".
	Issuer string `json:"issuer"`

	// Message corresponds to the JSON schema field "message".
	Message string `json:"message"`

	// Stopped corresponds to the JSON schema field "stopped".
	Stopped bool `json:"stopped"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *StopWorkloadResponse) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["id"]; raw != nil && !ok {
		return fmt.Errorf("field id in StopWorkloadResponse: required")
	}
	if _, ok := raw["issuer"]; raw != nil && !ok {
		return fmt.Errorf("field issuer in StopWorkloadResponse: required")
	}
	if _, ok := raw["message"]; raw != nil && !ok {
		return fmt.Errorf("field message in StopWorkloadResponse: required")
	}
	if _, ok := raw["stopped"]; raw != nil && !ok {
		return fmt.Errorf("field stopped in StopWorkloadResponse: required")
	}
	type Plain StopWorkloadResponse
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = StopWorkloadResponse(plain)
	return nil
}

type Workload struct {
	// The unique identifier of the workload
	Id string `json:"id"`

	// The name of the workload
	Name string `json:"name"`

	// The runtime of the workload
	Runtime string `json:"runtime"`

	// The start time of the workload
	StartTime string `json:"start_time"`

	// The runtype/lifecycle of the workload
	WorkloadRuntype string `json:"workload_runtype"`

	// The state of the workload
	WorkloadState string `json:"workload_state"`

	// The type of the workload
	WorkloadType string `json:"workload_type"`
}

type WorkloadPingMachineSummary struct {
	// Id corresponds to the JSON schema field "id".
	Id string `json:"id"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name"`

	// Namespace corresponds to the JSON schema field "namespace".
	Namespace string `json:"namespace"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *WorkloadPingMachineSummary) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["id"]; raw != nil && !ok {
		return fmt.Errorf("field id in WorkloadPingMachineSummary: required")
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in WorkloadPingMachineSummary: required")
	}
	if _, ok := raw["namespace"]; raw != nil && !ok {
		return fmt.Errorf("field namespace in WorkloadPingMachineSummary: required")
	}
	type Plain WorkloadPingMachineSummary
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = WorkloadPingMachineSummary(plain)
	return nil
}

type WorkloadPingResponse struct {
	// WorkloadSummary corresponds to the JSON schema field "workload_summary".
	WorkloadSummary *Workload `json:"workload_summary,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *Workload) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["id"]; raw != nil && !ok {
		return fmt.Errorf("field id in Workload: required")
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in Workload: required")
	}
	if _, ok := raw["runtime"]; raw != nil && !ok {
		return fmt.Errorf("field runtime in Workload: required")
	}
	if _, ok := raw["start_time"]; raw != nil && !ok {
		return fmt.Errorf("field start_time in Workload: required")
	}
	if _, ok := raw["workload_runtype"]; raw != nil && !ok {
		return fmt.Errorf("field workload_runtype in Workload: required")
	}
	if _, ok := raw["workload_state"]; raw != nil && !ok {
		return fmt.Errorf("field workload_state in Workload: required")
	}
	if _, ok := raw["workload_type"]; raw != nil && !ok {
		return fmt.Errorf("field workload_type in Workload: required")
	}
	type Plain Workload
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = Workload(plain)
	return nil
}
