// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

import "encoding/json"
import "fmt"

type LameduckRequestJson struct {
	// Time delay before lameduck mode is set
	Delay string `json:"delay"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *LameduckRequestJson) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["delay"]; raw != nil && !ok {
		return fmt.Errorf("field delay in LameduckRequestJson: required")
	}
	type Plain LameduckRequestJson
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = LameduckRequestJson(plain)
	return nil
}
