// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

import "encoding/json"
import "fmt"
import "reflect"

type AuctionRequestJson struct {
	// The architecture auction for
	Arch *AuctionRequestJsonArch `json:"arch,omitempty" yaml:"arch,omitempty" mapstructure:"arch,omitempty"`

	// The operating system to auction for
	Os *AuctionRequestJsonOs `json:"os,omitempty" yaml:"os,omitempty" mapstructure:"os,omitempty"`

	// A list of tags to associate with the node during auction
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty" mapstructure:"tags,omitempty"`

	// The type of workload to auction for
	WorkloadType []string `json:"workload_type,omitempty" yaml:"workload_type,omitempty" mapstructure:"workload_type,omitempty"`
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
