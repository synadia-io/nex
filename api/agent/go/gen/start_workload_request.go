// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

import "encoding/json"
import "fmt"

type StartWorkloadRequestJson struct {
	// Command line arguments to be passed when the workload is a native/service type
	Argv []string `json:"argv,omitempty"`

	// A map containing environment variables, applicable for native workload types
	Env StartWorkloadRequestJsonEnv `json:"env,omitempty"`

	// A hex encoded SHA-256 hash of the artifact file bytes
	Hash string `json:"hash"`

	// Name of the workload
	Name string `json:"name"`

	// Namespace of the workload
	Namespace string `json:"namespace"`

	// Byte size of the workload artifact
	TotalBytes int `json:"totalBytes"`

	// A list of trigger subjects for the workload, if applicable. Note these are NOT
	// subscribed to by the agent, only used for information and validation
	TriggerSubjects []string `json:"triggerSubjects,omitempty"`

	// The unique identifier of the workload to start.
	WorkloadId string `json:"workloadId"`

	// Type of the workload
	WorkloadType string `json:"workloadType"`
}

// A map containing environment variables, applicable for native workload types
type StartWorkloadRequestJsonEnv map[string]interface{}

// UnmarshalJSON implements json.Unmarshaler.
func (j *StartWorkloadRequestJson) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["hash"]; raw != nil && !ok {
		return fmt.Errorf("field hash in StartWorkloadRequestJson: required")
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in StartWorkloadRequestJson: required")
	}
	if _, ok := raw["namespace"]; raw != nil && !ok {
		return fmt.Errorf("field namespace in StartWorkloadRequestJson: required")
	}
	if _, ok := raw["totalBytes"]; raw != nil && !ok {
		return fmt.Errorf("field totalBytes in StartWorkloadRequestJson: required")
	}
	if _, ok := raw["workloadId"]; raw != nil && !ok {
		return fmt.Errorf("field workloadId in StartWorkloadRequestJson: required")
	}
	if _, ok := raw["workloadType"]; raw != nil && !ok {
		return fmt.Errorf("field workloadType in StartWorkloadRequestJson: required")
	}
	type Plain StartWorkloadRequestJson
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	if 1 > plain.TotalBytes {
		return fmt.Errorf("field %s: must be >= %v", "totalBytes", 1)
	}
	*j = StartWorkloadRequestJson(plain)
	return nil
}
