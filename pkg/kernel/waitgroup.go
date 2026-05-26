package kernel

import "sync"

// WaitGroup coordinates a set of agents working together.
type WaitGroup struct {
	mu      sync.Mutex
	cond    *sync.Cond
	pending map[AgentID]bool
	results []AgentResult
}

// AgentResult holds the outcome of a waited-upon agent.
type AgentResult struct {
	AgentID AgentID
	Success bool
	Error   string
}

// NewWaitGroup creates a coordinator.
func NewWaitGroup() *WaitGroup {
	wg := &WaitGroup{pending: make(map[AgentID]bool)}
	wg.cond = sync.NewCond(&wg.mu)
	return wg
}

// Add registers an agent that must complete.
func (wg *WaitGroup) Add(agentID AgentID) {
	wg.mu.Lock()
	defer wg.mu.Unlock()
	wg.pending[agentID] = true
}

// Done marks an agent as complete and broadcasts to waiters.
func (wg *WaitGroup) Done(agentID AgentID, success bool, errMsg string) {
	wg.mu.Lock()
	defer wg.mu.Unlock()
	delete(wg.pending, agentID)
	wg.results = append(wg.results, AgentResult{AgentID: agentID, Success: success, Error: errMsg})
	if len(wg.pending) == 0 {
		wg.cond.Broadcast()
	}
}

// Wait blocks until all agents have called Done.
func (wg *WaitGroup) Wait() []AgentResult {
	wg.mu.Lock()
	defer wg.mu.Unlock()
	for len(wg.pending) > 0 {
		wg.cond.Wait()
	}
	return wg.results
}
