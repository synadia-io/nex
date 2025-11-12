package models

import (
	"encoding/json"
	"fmt"
)

const NexfileName string = "Nexfile"

type Nexfile struct {
	Name         string            `json:"name" yaml:"name"`
	Description  string            `json:"description" yaml:"description"`
	AuctionTags  map[string]string `json:"tags" yaml:"tags"`
	Type         string            `json:"type" yaml:"type"`
	Lifecycle    string            `json:"lifecycle" yaml:"lifecycle"`
	StartRequest any               `json:"start_request" yaml:"start_request"`
}

func (j *Nexfile) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in Nexfile: required")
	}
	if _, ok := raw["type"]; raw != nil && !ok {
		return fmt.Errorf("field type in Nexfile: required")
	}
	if _, ok := raw["lifecycle"]; raw != nil && !ok {
		return fmt.Errorf("field lifecycle in Nexfile: required")
	}
	if _, ok := raw["start_request"]; raw != nil && !ok {
		return fmt.Errorf("field start_request in Nexfile: required")
	}
	type Plain Nexfile
	var plain Plain
	if err := json.Unmarshal(b, &plain); err != nil {
		return err
	}
	*j = Nexfile(plain)
	return nil
}
