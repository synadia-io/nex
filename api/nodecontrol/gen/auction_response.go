// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

import "encoding/json"
import "fmt"

type AuctionResponseJson struct {
	// The unique identifier of the node
	BidderId string `json:"bidder_id" yaml:"bidder_id" mapstructure:"bidder_id"`

	// The name of the nexus
	Nexus string `json:"nexus" yaml:"nexus" mapstructure:"nexus"`

	// The number of agents running and their workload counts
	Status AuctionResponseJsonStatus `json:"status" yaml:"status" mapstructure:"status"`

	// Tags corresponds to the JSON schema field "tags".
	Tags AuctionResponseJsonTags `json:"tags" yaml:"tags" mapstructure:"tags"`

	// The target nodes xkey
	TargetXkey string `json:"target_xkey" yaml:"target_xkey" mapstructure:"target_xkey"`

	// The uptime of the node
	Uptime string `json:"uptime" yaml:"uptime" mapstructure:"uptime"`

	// The version of the node
	Version string `json:"version" yaml:"version" mapstructure:"version"`
}

// The number of agents running and their workload counts
type AuctionResponseJsonStatus struct {
	// Status corresponds to the JSON schema field "status".
	Status AuctionResponseJsonStatusStatus `json:"status,omitempty" yaml:"status,omitempty" mapstructure:"status,omitempty"`
}

type AuctionResponseJsonStatusStatus map[string]int

type AuctionResponseJsonTags struct {
	// Tags corresponds to the JSON schema field "tags".
	Tags AuctionResponseJsonTagsTags `json:"tags,omitempty" yaml:"tags,omitempty" mapstructure:"tags,omitempty"`
}

type AuctionResponseJsonTagsTags map[string]string

// UnmarshalJSON implements json.Unmarshaler.
func (j *AuctionResponseJson) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["bidder_id"]; raw != nil && !ok {
		return fmt.Errorf("field bidder_id in AuctionResponseJson: required")
	}
	if _, ok := raw["nexus"]; raw != nil && !ok {
		return fmt.Errorf("field nexus in AuctionResponseJson: required")
	}
	if _, ok := raw["status"]; raw != nil && !ok {
		return fmt.Errorf("field status in AuctionResponseJson: required")
	}
	if _, ok := raw["tags"]; raw != nil && !ok {
		return fmt.Errorf("field tags in AuctionResponseJson: required")
	}
	if _, ok := raw["target_xkey"]; raw != nil && !ok {
		return fmt.Errorf("field target_xkey in AuctionResponseJson: required")
	}
	if _, ok := raw["uptime"]; raw != nil && !ok {
		return fmt.Errorf("field uptime in AuctionResponseJson: required")
	}
	if _, ok := raw["version"]; raw != nil && !ok {
		return fmt.Errorf("field version in AuctionResponseJson: required")
	}
	type Plain AuctionResponseJson
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = AuctionResponseJson(plain)
	return nil
}
