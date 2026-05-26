package team

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// SetTopology validates and stores the team topology. Every agent referenced
// in t.Agents must be registered.
func (s *TeamStore) SetTopology(t Topology) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t.Type == "" {
		return fmt.Errorf("topology type is required")
	}

	for _, name := range t.Agents {
		if _, ok := s.agents[name]; !ok {
			return fmt.Errorf("agent %q is not registered", name)
		}
	}

	// Deep copy to avoid aliasing caller's maps/slices.
	s.topology.Type = t.Type
	s.topology.Agents = copySlice(t.Agents)
	if t.Options != nil {
		s.topology.Options = make(map[string]string, len(t.Options))
		for k, v := range t.Options {
			s.topology.Options[k] = v
		}
	} else {
		s.topology.Options = nil
	}
	return nil
}

// GetTopology returns a copy of the current topology.
func (s *TeamStore) GetTopology() Topology {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return Topology{
		Type:    s.topology.Type,
		Agents:  copySlice(s.topology.Agents),
		Options: copyMap(s.topology.Options),
	}
}

// TopologyYAML serialises the current topology as YAML.
func (s *TeamStore) TopologyYAML() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return yaml.Marshal(s.topology)
}

// ParseTopologyYAML parses YAML and applies it via SetTopology.
func (s *TeamStore) ParseTopologyYAML(data []byte) error {
	var t Topology
	if err := yaml.Unmarshal(data, &t); err != nil {
		return fmt.Errorf("failed to parse topology YAML: %w", err)
	}
	return s.SetTopology(t)
}

// StatusText produces a human-readable summary of the team's current state:
// topology, agents, and task counts grouped by status.
func (s *TeamStore) StatusText() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("=== TEAM STATUS ===\n\n")

	sb.WriteString(fmt.Sprintf("Topology: %s\n", s.topology.Type))
	sb.WriteString(fmt.Sprintf("Agents in topology: %v\n\n", s.topology.Agents))

	sb.WriteString("--- Agents ---\n")
	for _, a := range s.agents {
		sb.WriteString(fmt.Sprintf("  [%s] %s (%s) - %s\n", a.Role, a.Name, a.Status, a.Workspace))
	}

	sb.WriteString("\n--- Tasks ---\n")
	counts := make(map[TaskStatus]int)
	for _, t := range s.tasks {
		counts[t.Status]++
	}
	order := []TaskStatus{TaskCreated, TaskAssigned, TaskInProgress, TaskReview, TaskDone, TaskRejected, TaskBlocked}
	for _, st := range order {
		if c, ok := counts[st]; ok {
			sb.WriteString(fmt.Sprintf("  %s: %d\n", st, c))
		}
	}

	return []byte(sb.String()), nil
}
