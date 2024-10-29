// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

import "encoding/json"
import "fmt"

type NodePingResponseJson struct {
	// The name of the nexus - if assigned
	Nexus string `json:"nexus" yaml:"nexus" mapstructure:"nexus"`

	// The unique identifier of the node
	NodeId string `json:"node_id" yaml:"node_id" mapstructure:"node_id"`

	// The number of agents running with workload count
	RunningAgents NodePingResponseJsonRunningAgents `json:"running_agents" yaml:"running_agents" mapstructure:"running_agents"`

	// Tags corresponds to the JSON schema field "tags".
	Tags NodePingResponseJsonTags `json:"tags" yaml:"tags" mapstructure:"tags"`

	// The target nodes xkey
	TargetXkey string `json:"target_xkey" yaml:"target_xkey" mapstructure:"target_xkey"`

	// The uptime of the node
	Uptime string `json:"uptime" yaml:"uptime" mapstructure:"uptime"`

	// The version of the node
	Version string `json:"version" yaml:"version" mapstructure:"version"`
}

// The number of agents running with workload count
type NodePingResponseJsonRunningAgents struct {
	// Status corresponds to the JSON schema field "status".
	Status NodePingResponseJsonRunningAgentsStatus `json:"status,omitempty" yaml:"status,omitempty" mapstructure:"status,omitempty"`
}

type NodePingResponseJsonRunningAgentsStatus map[string]int

type NodePingResponseJsonTags struct {
	// Tags corresponds to the JSON schema field "tags".
	Tags NodePingResponseJsonTagsTags `json:"tags,omitempty" yaml:"tags,omitempty" mapstructure:"tags,omitempty"`
}

type NodePingResponseJsonTagsTags map[string]string

// UnmarshalJSON implements json.Unmarshaler.
func (j *NodePingResponseJson) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["nexus"]; raw != nil && !ok {
		return fmt.Errorf("field nexus in NodePingResponseJson: required")
	}
	if _, ok := raw["node_id"]; raw != nil && !ok {
		return fmt.Errorf("field node_id in NodePingResponseJson: required")
	}
	if _, ok := raw["running_agents"]; raw != nil && !ok {
		return fmt.Errorf("field running_agents in NodePingResponseJson: required")
	}
	if _, ok := raw["tags"]; raw != nil && !ok {
		return fmt.Errorf("field tags in NodePingResponseJson: required")
	}
	if _, ok := raw["target_xkey"]; raw != nil && !ok {
		return fmt.Errorf("field target_xkey in NodePingResponseJson: required")
	}
	if _, ok := raw["uptime"]; raw != nil && !ok {
		return fmt.Errorf("field uptime in NodePingResponseJson: required")
	}
	if _, ok := raw["version"]; raw != nil && !ok {
		return fmt.Errorf("field version in NodePingResponseJson: required")
	}
	type Plain NodePingResponseJson
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = NodePingResponseJson(plain)
	return nil
}
