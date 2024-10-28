// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

import "encoding/json"
import "fmt"
import "reflect"

type AuctionRequestJson struct {
	// The type of agent to auction for
	AgentType []NexWorkload `json:"agent_type,omitempty" yaml:"agent_type,omitempty" mapstructure:"agent_type,omitempty"`

	// Specify a required architecture for the auction
	Arch *AuctionRequestJsonArch `json:"arch,omitempty" yaml:"arch,omitempty" mapstructure:"arch,omitempty"`

	// Specify a required operating system for the auction
	Os *AuctionRequestJsonOs `json:"os,omitempty" yaml:"os,omitempty" mapstructure:"os,omitempty"`

	// A list of tags to associate with the node during auction. To be returned, node
	// must satisfy ALL tags
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty" mapstructure:"tags,omitempty"`
}

type AuctionRequestJsonArch string

const AuctionRequestJsonArchAmd64 AuctionRequestJsonArch = "amd64"
const AuctionRequestJsonArchArm64 AuctionRequestJsonArch = "arm64"

var enumValues_AuctionRequestJsonArch = []interface{}{
	"amd64",
	"arm64",
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *AuctionRequestJsonArch) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	var ok bool
	for _, expected := range enumValues_AuctionRequestJsonArch {
		if reflect.DeepEqual(v, expected) {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("invalid value (expected one of %#v): %#v", enumValues_AuctionRequestJsonArch, v)
	}
	*j = AuctionRequestJsonArch(v)
	return nil
}

type AuctionRequestJsonOs string

const AuctionRequestJsonOsDarwin AuctionRequestJsonOs = "darwin"
const AuctionRequestJsonOsLinux AuctionRequestJsonOs = "linux"
const AuctionRequestJsonOsWindows AuctionRequestJsonOs = "windows"

var enumValues_AuctionRequestJsonOs = []interface{}{
	"linux",
	"darwin",
	"windows",
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *AuctionRequestJsonOs) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	var ok bool
	for _, expected := range enumValues_AuctionRequestJsonOs {
		if reflect.DeepEqual(v, expected) {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("invalid value (expected one of %#v): %#v", enumValues_AuctionRequestJsonOs, v)
	}
	*j = AuctionRequestJsonOs(v)
	return nil
}

type NexWorkload string
