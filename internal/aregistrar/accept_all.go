package aregistrar

import "github.com/synadia-labs/nex/models"

var _ models.AgentRegistrar = (*AllowAllRegistrar)(nil)

// AllowAllRegistrar is an implementation of models.AgentRegistrar that allows
// all registrations, both remote initilization and agent registration without any
// checks or restrictions.
type AllowAllRegistrar struct{}

func (a *AllowAllRegistrar) RegisterRemoteInit(_ *models.RegisterRemoteAgentRequest) error {
	return nil
}

func (a *AllowAllRegistrar) RegisterAgent(_ *models.RegisterAgentRequest) error {
	return nil
}
