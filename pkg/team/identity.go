package team

import (
	"errors"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// TeamStore is the central, thread-safe data store for the team package.
// It holds agents, tasks, messages, shared contexts, and the topology.
type TeamStore struct {
	mu       sync.RWMutex
	agents   map[string]*AgentInfo
	tasks    map[string]*Task
	messages map[string][]*Message // inbox, keyed by recipient name
	outbox   map[string][]*Message // outbox, keyed by sender name
	contexts []*SharedContext
	topology Topology
}

// NewTeamStore returns an initialized TeamStore.
func NewTeamStore() *TeamStore {
	return &TeamStore{
		agents:   make(map[string]*AgentInfo),
		tasks:    make(map[string]*Task),
		messages: make(map[string][]*Message),
		outbox:   make(map[string][]*Message),
		contexts: make([]*SharedContext, 0),
	}
}

// RegisterAgent adds or updates an agent in the team. If info.ID is empty a
// new UUID is generated. If info.Name already exists the entry is replaced.
func (s *TeamStore) RegisterAgent(info AgentInfo) error {
	if info.Name == "" {
		return errors.New("agent name cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if info.ID == "" {
		info.ID = newUUID()
	}
	if info.Registered.IsZero() {
		info.Registered = time.Now()
	}

	s.agents[info.Name] = &info
	return nil
}

// GetAgent returns a copy of the AgentInfo for the given name.
func (s *TeamStore) GetAgent(name string) (*AgentInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, ok := s.agents[name]
	if !ok {
		return nil, false
	}
	cp := *a
	return &cp, true
}

// ListAgents returns copies of all registered agents.
func (s *TeamStore) ListAgents() []*AgentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AgentInfo, 0, len(s.agents))
	for _, a := range s.agents {
		cp := *a
		result = append(result, &cp)
	}
	return result
}

// RosterYAML serialises the agent roster as YAML.
func (s *TeamStore) RosterYAML() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type rosterDoc struct {
		Agents []*AgentInfo `yaml:"agents"`
	}
	agents := make([]*AgentInfo, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, a)
	}
	return yaml.Marshal(rosterDoc{Agents: agents})
}
