package internal

import (
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/synadia-labs/nex/models"
)

type (
	AgentRegistration struct {
		ID              string                       `json:"id"`
		RegisterRequest *models.RegisterAgentRequest `json:"original_request"`
		Schema          *jsonschema.Schema           `json:"-"`

		// Only used in remote agent registration
		PubSigningKey string `json:"-"`
	}
	AgentRegistrations struct {
		rwLock        sync.RWMutex
		Registrations map[string]*AgentRegistration `json:"registrations"`
	}
)

func NewAgentRegistrations() *AgentRegistrations {
	return &AgentRegistrations{
		rwLock:        sync.RWMutex{},
		Registrations: make(map[string]*AgentRegistration), // map[agentID]*AgentRegistration
	}
}

func (ar *AgentRegistrations) Count() int {
	ar.rwLock.RLock()
	defer ar.rwLock.RUnlock()
	return len(ar.Registrations)
}

func (ar *AgentRegistrations) String() string {
	if ar == nil || ar.Count() == 0 {
		return ""
	}
	sb := strings.Builder{}

	i := 0

	ar.rwLock.RLock()
	defer ar.rwLock.RUnlock()
	for k, v := range ar.Registrations {
		sb.WriteString(fmt.Sprintf("%s [%s]", k, v.RegisterRequest.RegisterType))
		if i < len(ar.Registrations)-1 {
			sb.WriteString(", ")
		}
		i++
	}
	return sb.String()
}

func (ar *AgentRegistrations) GetByRegisterType(registerType string) (*AgentRegistration, error) {
	ar.rwLock.RLock()
	defer ar.rwLock.RUnlock()

	for _, reg := range ar.Registrations {
		if reg.RegisterRequest.RegisterType == registerType {
			return reg, nil
		}
	}

	return nil, fmt.Errorf("no agent registrations found for type: %s", registerType)
}

func (ar *AgentRegistrations) GetByRegisterName(registerName string) (*AgentRegistration, error) {
	ar.rwLock.RLock()
	defer ar.rwLock.RUnlock()

	for _, reg := range ar.Registrations {
		if reg.RegisterRequest.Name == registerName {
			return reg, nil
		}
	}

	return nil, fmt.Errorf("no agents registered with name: %s", registerName)
}

func (ar *AgentRegistrations) Add(reg *AgentRegistration) error {
	ar.rwLock.Lock()
	defer ar.rwLock.Unlock()
	if reg == nil {
		return fmt.Errorf("failed to add a <nil> registration")
	}
	if reg.ID == "" {
		return fmt.Errorf("failed to register agent with empty ID: %+v", reg)
	}
	if _, exists := ar.Registrations[reg.ID]; exists {
		return fmt.Errorf("agent with ID %s is already registered", reg.ID)
	}

	ar.Registrations[reg.ID] = reg
	return nil
}
