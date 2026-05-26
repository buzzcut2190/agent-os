package kernel

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LifecycleManager manages agent creation, state transitions, and termination.
type LifecycleManager struct {
	mu     sync.RWMutex
	agents map[AgentID]*Agent
}

// NewLifecycleManager creates a new lifecycle manager.
func NewLifecycleManager() *LifecycleManager {
	return &LifecycleManager{
		agents: make(map[AgentID]*Agent),
	}
}

// Spawn creates a new agent of the given type with the specified config.
func (m *LifecycleManager) Spawn(agentType AgentType, config AgentConfig) (AgentID, error) {
	if agentType == "" {
		return "", fmt.Errorf("agent type is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	id := AgentID(uuid.New().String()[:8])
	now := time.Now()
	agent := &Agent{
		ID:        id,
		Type:      agentType,
		State:     AgentCreated,
		CreatedAt: now,
		UpdatedAt: now,
		Config:    config,
		Resources: ResourceUsage{TokensLimit: config.MaxTokens},
	}
	m.agents[id] = agent
	return id, nil
}

// Get returns an agent by ID.
func (m *LifecycleManager) Get(id AgentID) (*Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.agents[id]
	return a, ok
}

// List returns agents optionally filtered by state.
func (m *LifecycleManager) List(filter AgentState) []*Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Agent
	for _, a := range m.agents {
		if filter == "" || a.State == filter {
			cp := *a
			result = append(result, &cp)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

// Suspend pauses an agent.
func (m *LifecycleManager) Suspend(id AgentID) error {
	return m.transition(id, AgentRunning, AgentSuspended)
}

// Resume wakes a suspended agent.
func (m *LifecycleManager) Resume(id AgentID) error {
	return m.transition(id, AgentSuspended, AgentRunning)
}

// Kill terminates an agent.
func (m *LifecycleManager) Kill(id AgentID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.agents[id]
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}
	a.State = AgentTerminated
	a.UpdatedAt = time.Now()
	return nil
}

// KillAll terminates all agents.
func (m *LifecycleManager) KillAll() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, a := range m.agents {
		if a.State != AgentTerminated && a.State != AgentFailed {
			a.State = AgentTerminated
			a.UpdatedAt = time.Now()
			count++
		}
	}
	return count
}

// Run transitions an agent from created → running.
func (m *LifecycleManager) Run(id AgentID) error {
	return m.transition(id, AgentCreated, AgentRunning)
}

// transition safely moves an agent between states.
func (m *LifecycleManager) transition(id AgentID, from, to AgentState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.agents[id]
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}
	if a.State != from {
		return fmt.Errorf("agent %s is %s, expected %s", id, a.State, from)
	}
	a.State = to
	a.UpdatedAt = time.Now()
	return nil
}

// Count returns the number of agents in a given state.
func (m *LifecycleManager) Count(state AgentState) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, a := range m.agents {
		if state == "" || a.State == state {
			n++
		}
	}
	return n
}

// ActiveCount returns how many agents are currently running.
func (m *LifecycleManager) ActiveCount() int {
	return m.Count(AgentRunning)
}
