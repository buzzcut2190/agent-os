package daemon

import (
	"context"
	"sync"
	"time"

	"github.com/agent-os/agent-os/pkg/kernel"
)

// MiningTask defines a background task that runs when the system is idle.
type MiningTask struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"` // 1 (lowest) - 10 (highest)
	Interval time.Duration `json:"interval"`
	Action   string `json:"action"` // agent type to spawn

	lastRun time.Time
}

// MinerStats reports background miner activity.
type MinerStats struct {
	TasksRun    int           `json:"tasks_run"`
	LastRun     time.Time     `json:"last_run"`
	IdleTime    time.Duration `json:"idle_time"`
	CurrentlyIdle bool        `json:"currently_idle"`
}

// BackgroundMiner runs low-priority background tasks only when the system
// has idle capacity (active agents below 30% of max).
type BackgroundMiner struct {
	mu        sync.RWMutex
	kernel    *kernel.LifecycleManager
	tasks     []*MiningTask
	interval  time.Duration
	maxAgents int
	stats     MinerStats
}

// NewBackgroundMiner creates a miner with default tasks loaded.
func NewBackgroundMiner(k *kernel.LifecycleManager, interval time.Duration, maxAgents int) *BackgroundMiner {
	m := &BackgroundMiner{
		kernel:    k,
		interval:  interval,
		maxAgents: maxAgents,
	}
	m.loadDefaults()
	return m
}

// loadDefaults seeds the miner with standard background tasks.
func (m *BackgroundMiner) loadDefaults() {
	m.tasks = []*MiningTask{
		{Name: "analyze-code-quality", Priority: 3, Interval: 1 * time.Hour, Action: "code-reviewer"},
		{Name: "consolidate-memory", Priority: 5, Interval: 2 * time.Hour, Action: "orchestrator"},
	}
}

// Start begins the mining loop, checking idleness on each tick.
func (m *BackgroundMiner) Start(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.tick()
		}
	}
}

// tick evaluates idleness and runs eligible background tasks.
func (m *BackgroundMiner) tick() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.IsIdle() {
		return
	}

	now := time.Now()
	for _, t := range m.tasks {
		if now.Sub(t.lastRun) >= t.Interval {
			t.lastRun = now
			// In production: kernel.Spawn(AgentType(t.Action), config).
			m.stats.TasksRun++
			m.stats.LastRun = now
		}
	}
}

// IsIdle returns true when active agent count is below 30% of max.
func (m *BackgroundMiner) IsIdle() bool {
	if m.maxAgents <= 0 {
		return false
	}
	active := m.kernel.ActiveCount()
	idle := float64(active)/float64(m.maxAgents) < 0.30
	if idle {
		m.stats.CurrentlyIdle = true
	} else {
		m.stats.CurrentlyIdle = false
	}
	return idle
}

// Stats returns a snapshot of current mining statistics.
func (m *BackgroundMiner) Stats() MinerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

// AddTask registers a new background mining task.
func (m *BackgroundMiner) AddTask(task *MiningTask) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks = append(m.tasks, task)
}

// RemoveTask removes a task by name.
func (m *BackgroundMiner) RemoveTask(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.tasks {
		if t.Name == name {
			m.tasks = append(m.tasks[:i], m.tasks[i+1:]...)
			return
		}
	}
}
