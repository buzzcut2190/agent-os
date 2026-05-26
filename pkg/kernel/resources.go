package kernel

import (
	"fmt"
	"sync"
)

// ResourceManager tracks and enforces resource limits.
type ResourceManager struct {
	mu      sync.RWMutex
	limits  ResourceLimits
	perAgent map[AgentID]ResourceUsage
}

// NewResourceManager creates a resource manager with the given limits.
func NewResourceManager(limits ResourceLimits) *ResourceManager {
	return &ResourceManager{
		limits:   limits,
		perAgent: make(map[AgentID]ResourceUsage),
	}
}

// Allocate reserves resources for an agent.
func (rm *ResourceManager) Allocate(agentID AgentID, requested ResourceUsage) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	total := rm.totalUsedLocked() + requested.TokensUsed
	if total > rm.limits.MaxTotalTokens {
		return fmt.Errorf("%w: token limit exceeded (%d/%d)", ErrResourceExhausted, total, rm.limits.MaxTotalTokens)
	}
	active := rm.activeCountLocked()
	if active >= rm.limits.MaxAgents {
		return fmt.Errorf("%w: agent limit exceeded (%d/%d)", ErrResourceExhausted, active, rm.limits.MaxAgents)
	}
	if requested.TokensUsed > rm.limits.MaxTokensPerAgent {
		return fmt.Errorf("%w: per-agent token limit exceeded", ErrResourceExhausted)
	}
	rm.perAgent[agentID] = requested
	return nil
}

// Release frees resources held by an agent.
func (rm *ResourceManager) Release(agentID AgentID) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.perAgent, agentID)
	return nil
}

// Usage returns a summary of current resource utilization.
func (rm *ResourceManager) Usage() ResourceSummary {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return ResourceSummary{
		ActiveAgents: rm.activeCountLocked(),
		TotalAgents:  len(rm.perAgent),
		TokensUsed:   rm.totalUsedLocked(),
		TokensLimit:  rm.limits.MaxTotalTokens,
		ByType:       make(map[AgentType]int),
	}
}

// CanAllocate checks whether a request fits within current limits.
func (rm *ResourceManager) CanAllocate(requested ResourceUsage) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.activeCountLocked() < rm.limits.MaxAgents &&
		rm.totalUsedLocked()+requested.TokensUsed <= rm.limits.MaxTotalTokens
}

func (rm *ResourceManager) totalUsedLocked() int {
	total := 0
	for _, u := range rm.perAgent {
		total += u.TokensUsed
	}
	return total
}

func (rm *ResourceManager) activeCountLocked() int {
	return len(rm.perAgent)
}

// ErrResourceExhausted is returned when limits are exceeded.
var ErrResourceExhausted = fmt.Errorf("resource exhausted")
