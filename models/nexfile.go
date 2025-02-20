package models

const NexfileName string = "Nexfile"

type Nexfile struct {
	Name         string            `json:"name" yaml:"name"`
	Description  string            `json:"description" yaml:"description"`
	AuctionTags  map[string]string `json:"tags" yaml:"tags"`
	Type         string            `json:"type" yaml:"type"`
	Lifecycle    string            `json:"lifecycle" yaml:"lifecycle"`
	StartRequest any               `json:"start_request" yaml:"start_request"`
}
