// Code generated by github.com/atombender/go-jsonschema, DO NOT EDIT.

package gen

type LameduckResponseJson struct {
	// The unique identifier of the node
	NodeId *string `json:"node_id,omitempty" yaml:"node_id,omitempty" mapstructure:"node_id,omitempty"`

	// Indicates if the node was successfully placed into lameduck mode
	Success *bool `json:"success,omitempty" yaml:"success,omitempty" mapstructure:"success,omitempty"`
}