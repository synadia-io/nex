// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

import "encoding/json"
import "fmt"

type NodeInfoResponseJson struct {
	// The name of the nexus - if assigned
	Nexus string `json:"nexus" yaml:"nexus" mapstructure:"nexus"`

	// The unique identifier of the node
	NodeId string `json:"node_id" yaml:"node_id" mapstructure:"node_id"`

	// Tags corresponds to the JSON schema field "tags".
	Tags NodeInfoResponseJsonTags `json:"tags" yaml:"tags" mapstructure:"tags"`

	// The target nodes xkey
	TargetXkey string `json:"target_xkey" yaml:"target_xkey" mapstructure:"target_xkey"`

	// The uptime of the node
	Uptime string `json:"uptime" yaml:"uptime" mapstructure:"uptime"`

	// The version of the node
	Version string `json:"version" yaml:"version" mapstructure:"version"`

	// WorkloadSummaries corresponds to the JSON schema field "workload_summaries".
	WorkloadSummaries []WorkloadSummary `json:"workload_summaries" yaml:"workload_summaries" mapstructure:"workload_summaries"`
}

type NodeInfoResponseJsonTags struct {
	// Tags corresponds to the JSON schema field "tags".
	Tags NodeInfoResponseJsonTagsTags `json:"tags,omitempty" yaml:"tags,omitempty" mapstructure:"tags,omitempty"`
}

type NodeInfoResponseJsonTagsTags map[string]string

// UnmarshalJSON implements json.Unmarshaler.
func (j *NodeInfoResponseJson) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["nexus"]; raw != nil && !ok {
		return fmt.Errorf("field nexus in NodeInfoResponseJson: required")
	}
	if _, ok := raw["node_id"]; raw != nil && !ok {
		return fmt.Errorf("field node_id in NodeInfoResponseJson: required")
	}
	if _, ok := raw["tags"]; raw != nil && !ok {
		return fmt.Errorf("field tags in NodeInfoResponseJson: required")
	}
	if _, ok := raw["target_xkey"]; raw != nil && !ok {
		return fmt.Errorf("field target_xkey in NodeInfoResponseJson: required")
	}
	if _, ok := raw["uptime"]; raw != nil && !ok {
		return fmt.Errorf("field uptime in NodeInfoResponseJson: required")
	}
	if _, ok := raw["version"]; raw != nil && !ok {
		return fmt.Errorf("field version in NodeInfoResponseJson: required")
	}
	if _, ok := raw["workload_summaries"]; raw != nil && !ok {
		return fmt.Errorf("field workload_summaries in NodeInfoResponseJson: required")
	}
	type Plain NodeInfoResponseJson
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = NodeInfoResponseJson(plain)
	return nil
}

type WorkloadSummary struct {
	// The unique identifier of the workload
	Id string `json:"id" yaml:"id" mapstructure:"id"`

	// The name of the workload
	Name string `json:"name" yaml:"name" mapstructure:"name"`

	// The runtime of the workload
	Runtime string `json:"runtime" yaml:"runtime" mapstructure:"runtime"`

	// The start time of the workload
	StartTime string `json:"start_time" yaml:"start_time" mapstructure:"start_time"`

	// The type of the workload
	WorkloadType string `json:"workload_type" yaml:"workload_type" mapstructure:"workload_type"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *WorkloadSummary) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["id"]; raw != nil && !ok {
		return fmt.Errorf("field id in WorkloadSummary: required")
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in WorkloadSummary: required")
	}
	if _, ok := raw["runtime"]; raw != nil && !ok {
		return fmt.Errorf("field runtime in WorkloadSummary: required")
	}
	if _, ok := raw["start_time"]; raw != nil && !ok {
		return fmt.Errorf("field start_time in WorkloadSummary: required")
	}
	if _, ok := raw["workload_type"]; raw != nil && !ok {
		return fmt.Errorf("field workload_type in WorkloadSummary: required")
	}
	type Plain WorkloadSummary
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = WorkloadSummary(plain)
	return nil
}