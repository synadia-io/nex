// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

import "encoding/json"
import "fmt"

type CloneWorkloadRequestJson struct {
	// NewTargetXkey corresponds to the JSON schema field "new_target_xkey".
	NewTargetXkey string `json:"new_target_xkey" yaml:"new_target_xkey" mapstructure:"new_target_xkey"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *CloneWorkloadRequestJson) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["new_target_xkey"]; raw != nil && !ok {
		return fmt.Errorf("field new_target_xkey in CloneWorkloadRequestJson: required")
	}
	type Plain CloneWorkloadRequestJson
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = CloneWorkloadRequestJson(plain)
	return nil
}
