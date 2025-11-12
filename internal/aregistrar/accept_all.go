package aregistrar

import "github.com/synadia-io/nex/models"

var _ models.AgentRegistrar = (*AllowAllRegistrar)(nil)

// AllowAllRegistrar is an implementation of models.AgentRegistrar that allows
// all registrations, both remote initilization and agent registration without any
// checks or restrictions.
type AllowAllRegistrar struct{}

func (a *AllowAllRegistrar) RegisterRemoteInit(_ map[string][]string, _ *models.RegisterRemoteAgentRequest) error {
	return nil
}

func (a *AllowAllRegistrar) RegisterAgent(_ map[string][]string, _ *models.RegisterAgentRequest) error {
	return nil
}
