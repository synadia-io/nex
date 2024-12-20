// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

import "encoding/json"
import "fmt"

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

type WorkloadPingResponseJson struct {
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
