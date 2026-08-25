package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/synadia-io/nex/models"
)

type AgentHealthStatus int

const (
	AgentHealthy      AgentHealthStatus = iota
	AgentShuttingDown                   // AgentShuttingDown indicates the agent is in the process of shutting down
	AgentDegraded                       // AgentDegraded indicates the agent is running but has some issues
	AgentOffline                        // AgentOffline indicates the agent is not reachable or not running
	AgentUnknown                        // AgentUnknown indicates the agent's health status is unknown because no heartbeat has been received
)

func (ahs AgentHealthStatus) String() string {
	switch ahs {
	case AgentHealthy:
		return "healthy"
	case AgentShuttingDown:
		return "shutting down"
	case AgentDegraded:
		return "degraded"
	case AgentOffline:
		return "offline"
	default:
		return "unknown"
	}
}

func (ahs AgentHealthStatus) Stoplight() string {
	switch ahs {
	case AgentHealthy:
		return "🟢"
	case AgentShuttingDown:
		return "🟠"
	case AgentDegraded:
		return "🟡"
	case AgentOffline:
		return "🔴"
	default:
		return "⚪️"
	}
}

type (
	AgentRegistration struct {
		ID                string                       `json:"id"`
		RegisterRequest   *models.RegisterAgentRequest `json:"register_request"`
		Schema            *jsonschema.Schema           `json:"-"`
		HealthStatus      AgentHealthStatus            `json:"health_status"`
		lastHeartbeat     time.Time                    `json:"-"`
		lastHeartbeatData models.AgentSummary          `json:"-"`
		rwLock            sync.RWMutex                 `json:"-"`
	}
	AgentRegistrations struct {
		ctx    context.Context `json:"-"`
		rwLock sync.RWMutex    `json:"-"`
		nc     *nats.Conn      `json:"-"`
		logger *slog.Logger    `json:"-"`
		nodeID string          `json:"-"`

		heartbeatSubs map[string]*nats.Subscription `json:"-"`
		Registrations map[string]*AgentRegistration `json:"registrations"`
	}
)

func NewAgentRegistrations(ctx context.Context, nodeID string, nc *nats.Conn, logger *slog.Logger) *AgentRegistrations {
	ar := &AgentRegistrations{
		ctx:           ctx,
		rwLock:        sync.RWMutex{},
		nodeID:        nodeID,
		logger:        logger,
		nc:            nc,
		heartbeatSubs: make(map[string]*nats.Subscription), // map[agentID]*nats.Subscription
		Registrations: make(map[string]*AgentRegistration), // map[agentID]*AgentRegistration
	}

	go ar.startHealthMonitor()

	return ar
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
	ar.rwLock.RLock()
	defer ar.rwLock.RUnlock()
	return ar.stringNoLock()
}

func (ar *AgentRegistrations) stringNoLock() string {
	sb := strings.Builder{}
	i := 0
	for k, v := range ar.Registrations {
		fmt.Fprintf(&sb, "%s [%s]", k, v.RegisterRequest.RegisterType)
		if i < len(ar.Registrations)-1 {
			sb.WriteString(", ")
		}
		i++
	}
	return sb.String()
}

func (ar *AgentRegistrations) AgentSummaries() []models.NodeAgentSummary {
	ar.rwLock.RLock()
	defer ar.rwLock.RUnlock()

	ret := make([]models.NodeAgentSummary, 0, len(ar.Registrations))

	for id, reg := range ar.Registrations {
		reg.rwLock.RLock()
		summary := models.NodeAgentSummary{
			AgentId:                id,
			AgentHealth:            reg.HealthStatus.String(),
			AgentHealthStoplight:   reg.HealthStatus.Stoplight(),
			AgentLastHeartbeatTime: reg.lastHeartbeat,
			AgentSummary:           reg.lastHeartbeatData,
		}
		reg.rwLock.RUnlock()
		ret = append(ret, summary)
	}

	return ret
}

func (ar *AgentRegistrations) GetByRegisterType(registerType string) (*AgentRegistration, error) {
	ar.rwLock.RLock()
	defer ar.rwLock.RUnlock()

	for _, reg := range ar.Registrations {
		reg.rwLock.RLock()
		if reg.RegisterRequest.RegisterType == registerType && reg.HealthStatus == AgentHealthy {
			reg.rwLock.RUnlock()
			return reg, nil
		}
		reg.rwLock.RUnlock()
	}

	return nil, fmt.Errorf("no agent registrations found for type: %s", registerType)
}

// GetByRegisterName prefers a Healthy match: while a crashed instance's
// registration lingers Offline (up to agentHardEvictAfter) beside its
// same-Name replacement, map iteration order would otherwise pick between
// corpse and replacement at random.
func (ar *AgentRegistrations) GetByRegisterName(registerName string) (*AgentRegistration, error) {
	ar.rwLock.RLock()
	defer ar.rwLock.RUnlock()

	var fallback *AgentRegistration
	for _, reg := range ar.Registrations {
		if reg.RegisterRequest.Name != registerName {
			continue
		}
		reg.rwLock.RLock()
		healthy := reg.HealthStatus == AgentHealthy
		reg.rwLock.RUnlock()
		if healthy {
			return reg, nil
		}
		if fallback == nil {
			fallback = reg
		}
	}
	if fallback != nil {
		return fallback, nil
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
		return fmt.Errorf("failed to register agent with empty ID")
	}
	if _, exists := ar.Registrations[reg.ID]; exists {
		return fmt.Errorf("agent with ID %s is already registered", reg.ID)
	}

	reg.rwLock = sync.RWMutex{}                   // Ensure the registration has its own lock
	reg.HealthStatus = AgentUnknown               // Default health status when adding a new registration
	reg.lastHeartbeatData = models.AgentSummary{} // Initialize last heartbeat data
	// Seed lastHeartbeat with the registration time so a just-added agent is
	// not immediately graded Offline (and evicted) before its first heartbeat
	// lands -- a zero time reads as "decades since last heartbeat".
	reg.lastHeartbeat = time.Now()
	go ar.startAgentHeartbeatMonitor(reg)

	ar.Registrations[reg.ID] = reg
	return nil
}

// Remove evicts a registration and tears down its heartbeat subscription. It
// is how a dead agent's ID is freed: without it an offline registration
// lingers forever and, because every agent instance now registers under a
// fresh ID, the map would grow without bound. Removing an entry does not
// disturb a handler that already holds the *AgentRegistration pointer -- Go
// keeps the struct alive for that reference; the entry simply stops being
// found by future lookups.
func (ar *AgentRegistrations) Remove(id string) {
	ar.rwLock.Lock()
	defer ar.rwLock.Unlock()

	if sub, ok := ar.heartbeatSubs[id]; ok && sub != nil {
		if err := sub.Unsubscribe(); err != nil {
			ar.logger.Error("failed to unsubscribe from agent heartbeat during eviction", slog.String("agent_id", id), slog.String("err", err.Error()))
		}
		delete(ar.heartbeatSubs, id)
	}
	delete(ar.Registrations, id)
}

func (ar *AgentRegistrations) startAgentHeartbeatMonitor(a *AgentRegistration) {
	sub, err := ar.nc.Subscribe(models.AgentAPIHeartbeatSubject(ar.nodeID, a.ID), func(msg *nats.Msg) {
		var agentHeartbeat models.AgentHeartbeat
		if err := json.Unmarshal(msg.Data, &agentHeartbeat); err != nil {
			ar.logger.Error("failed to unmarshal agent heartbeat", slog.String("agent_id", a.ID), slog.String("error", err.Error()))
			return
		}
		a.rwLock.Lock()
		a.HealthStatus = func() AgentHealthStatus {
			switch agentHeartbeat.Summary.State {
			case string(models.AgentStateRunning):
				return AgentHealthy
			case string(models.AgentStateError), string(models.AgentStateStarting):
				return AgentDegraded
			case string(models.AgentStateLameduck), string(models.AgentStateStopping):
				return AgentShuttingDown
			default:
				return AgentUnknown
			}
		}()
		a.lastHeartbeat = time.Now()
		a.lastHeartbeatData = agentHeartbeat.Summary
		a.rwLock.Unlock()
	})
	if err != nil {
		fmt.Printf("Error subscribing to agent heartbeat: %v\n", err)
		return
	}
	ar.rwLock.Lock()
	ar.heartbeatSubs[a.ID] = sub
	ar.rwLock.Unlock()
}

func (ar *AgentRegistrations) startHealthMonitor() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ar.ctx.Done():
			ar.logger.Info("Stopping agent health monitor")
			ar.rwLock.Lock()
			for _, sub := range ar.heartbeatSubs {
				if sub != nil {
					err := sub.Unsubscribe()
					if err != nil {
						ar.logger.Error("failed to unsubscribe from agent heartbeat", slog.String("error", err.Error()))
					}
				}
			}
			ar.heartbeatSubs = make(map[string]*nats.Subscription)
			ar.rwLock.Unlock()
			return
		case <-ticker.C:
			ar.sweep()
		}
	}
}

// sweep is one health-monitor pass: grade every registration by heartbeat
// age and evict the ones silent past the hard TTL. Extracted from the ticker
// loop so the policy is testable without waiting out real tickers.
//
// The grading is two-stage on purpose. Offline (30s) only EXCLUDES the agent
// from placement (GetByRegisterType filters on Healthy) -- it is fully
// recoverable, because the heartbeat subscription stays up and the next
// heartbeat re-grades the agent. Eviction is not recoverable: agents register
// exactly once at startup, so evicting a live-but-briefly-silent agent (a
// >30s NATS outage, a heartbeat starved behind a slow synchronous start)
// orphans it until its process restarts. Only silence past agentHardEvictAfter
// is treated as death.
func (ar *AgentRegistrations) sweep() {
	var dead []string
	ar.rwLock.RLock()
	for id, reg := range ar.Registrations {
		reg.rwLock.Lock()
		switch {
		case time.Since(reg.lastHeartbeat) > 10*time.Second && time.Since(reg.lastHeartbeat) <= 30*time.Second:
			reg.HealthStatus = AgentDegraded
		case time.Since(reg.lastHeartbeat) > 30*time.Second:
			reg.HealthStatus = AgentOffline
			if time.Since(reg.lastHeartbeat) > agentHardEvictAfter {
				dead = append(dead, id)
			}
		}
		reg.rwLock.Unlock()
	}
	ar.rwLock.RUnlock()

	// Evict the dead entries in a second pass, outside the read lock.
	// A crashed agent's replacement re-registers under a NEW id, so an
	// entry silent this long will never receive another heartbeat -- it is
	// pure garbage, and freeing it keeps the map bounded.
	for _, id := range dead {
		// Re-check under the reg lock: a heartbeat landing between the
		// grading pass and this eviction re-graded the agent alive, and
		// evicting it then would orphan a live agent permanently.
		ar.rwLock.RLock()
		reg := ar.Registrations[id]
		ar.rwLock.RUnlock()
		if reg != nil {
			reg.rwLock.RLock()
			revived := time.Since(reg.lastHeartbeat) <= agentHardEvictAfter
			reg.rwLock.RUnlock()
			if revived {
				continue
			}
		}
		ar.logger.Info("evicting dead agent registration", slog.String("agent_id", id))
		ar.Remove(id)
	}
}

// agentHardEvictAfter is how long an agent may be silent before its
// registration is evicted outright. Eviction is PERMANENT for the agent
// instance -- agents register exactly once at startup, so an evicted live
// agent has no way back until its process restarts -- which is why this is
// minutes, not the 30s Offline threshold: Offline already excludes the agent
// from placement, costs nothing to keep, and heals itself on the next
// heartbeat. Only an agent silent this long is treated as genuinely dead (its
// replacement registers under a new id) and dropped to keep the map bounded.
const agentHardEvictAfter = 10 * time.Minute
