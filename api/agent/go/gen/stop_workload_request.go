// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

import "encoding/json"
import "fmt"

type StopWorkloadRequestJson struct {
	// Indicates whether the stoppage should be immediate or graceful
	Immediate *bool `json:"immediate,omitempty"`

	// Optional reason for stopping the workload
	Reason *string `json:"reason,omitempty"`

	// The unique identifier of the workload to stop.
	WorkloadId string `json:"workloadId"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *StopWorkloadRequestJson) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["workloadId"]; raw != nil && !ok {
		return fmt.Errorf("field workloadId in StopWorkloadRequestJson: required")
	}
	type Plain StopWorkloadRequestJson
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = StopWorkloadRequestJson(plain)
	return nil
}
