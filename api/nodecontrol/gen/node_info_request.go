// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

import "encoding/json"
import "fmt"

type NodeInfoRequestJson struct {
	// Namespace of the node
	Namespace string `json:"namespace"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *NodeInfoRequestJson) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["namespace"]; raw != nil && !ok {
		return fmt.Errorf("field namespace in NodeInfoRequestJson: required")
	}
	type Plain NodeInfoRequestJson
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = NodeInfoRequestJson(plain)
	return nil
}
