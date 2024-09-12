// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package agentapi

import "encoding/json"
import "fmt"

type RegisterAgentRequestJson struct {
	// A user friendly description of the agent
	Description *string `json:"description,omitempty" yaml:"description,omitempty" mapstructure:"description,omitempty"`

	// The maximum number of workloads this agent can hold. 0 indicates unlimited
	MaxWorkloads *float64 `json:"max_workloads,omitempty" yaml:"max_workloads,omitempty" mapstructure:"max_workloads,omitempty"`

	// Name of the agent
	Name string `json:"name" yaml:"name" mapstructure:"name"`

	// Version of the agent
	Version string `json:"version" yaml:"version" mapstructure:"version"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *RegisterAgentRequestJson) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in RegisterAgentRequestJson: required")
	}
	if _, ok := raw["version"]; raw != nil && !ok {
		return fmt.Errorf("field version in RegisterAgentRequestJson: required")
	}
	type Plain RegisterAgentRequestJson
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = RegisterAgentRequestJson(plain)
	return nil
}
