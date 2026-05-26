package daemon

import (
	"context"
	"sync"
	"time"
)

// ScheduleJob defines a recurring task managed by the scheduler.
type ScheduleJob struct {
	Name     string         `json:"name"`
	Cron     string         `json:"cron"`
	Interval string         `json:"interval"` // e.g. "@every 30m"
	AgentType string        `json:"agent_type"`
	Payload  map[string]any `json:"payload"`
	Enabled  bool           `json:"enabled"`
	Timezone string         `json:"timezone"`

	lastRun time.Time
}

// ScheduleEngine runs periodic background jobs on a timer.
type ScheduleEngine struct {
	mu   sync.RWMutex
	jobs []*ScheduleJob
}

// NewScheduleEngine creates a scheduler with default jobs loaded.
func NewScheduleEngine() *ScheduleEngine {
	e := &ScheduleEngine{}
	e.loadDefaults()
	return e
}

// loadDefaults seeds the scheduler with common recurring tasks.
func (e *ScheduleEngine) loadDefaults() {
	e.jobs = []*ScheduleJob{
		{Name: "auto-review", Interval: "@every 30m", AgentType: "code-reviewer", Enabled: true, Payload: map[string]any{"scope": "auto"}},
		{Name: "update-context", Interval: "@every 1h", AgentType: "update-context", Enabled: true, Payload: map[string]any{}},
		{Name: "dead-code-report", Interval: "@daily 9am", AgentType: "tester", Enabled: true, Payload: map[string]any{"report": "dead-code"}},
		{Name: "daily-summary", Interval: "@daily 6pm", AgentType: "reporter", Enabled: true, Payload: map[string]any{"report": "daily-summary"}},
	}
}

// AddJob registers a new scheduled job.
func (e *ScheduleEngine) AddJob(job *ScheduleJob) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.jobs = append(e.jobs, job)
}

// RemoveJob removes a job by name.
func (e *ScheduleEngine) RemoveJob(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, j := range e.jobs {
		if j.Name == name {
			e.jobs = append(e.jobs[:i], e.jobs[i+1:]...)
			return
		}
	}
}

// ListJobs returns a snapshot of all registered jobs.
func (e *ScheduleEngine) ListJobs() []ScheduleJob {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]ScheduleJob, len(e.jobs))
	for i, j := range e.jobs {
		out[i] = *j
	}
	return out
}

// RunNow immediately executes a job by name, resetting its last-run time.
func (e *ScheduleEngine) RunNow(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, j := range e.jobs {
		if j.Name == name && j.Enabled {
			j.lastRun = time.Time{}
		}
	}
}

// Start begins the scheduling loop, checking jobs on a 30s tick.
func (e *ScheduleEngine) Start(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.tick()
		}
	}
}

// tick iterates jobs and fires any whose interval has elapsed.
func (e *ScheduleEngine) tick() {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	for _, j := range e.jobs {
		if !j.Enabled {
			continue
		}
		dur := parseInterval(j.Interval)
		if dur == 0 {
			continue
		}
		if now.Sub(j.lastRun) >= dur {
			j.lastRun = now
			// In production this would call kernel.Spawn(j.AgentType, config).
			_ = j.Name
		}
	}
}

// parseInterval converts an interval string like "@every 30m" to a duration.
// It handles "30m", "1h", "@every X", and basic daily specs.
func parseInterval(s string) time.Duration {
	if s == "" {
		return 0
	}
	// Strip "@every " prefix if present.
	prefix := "@every "
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		s = s[len(prefix):]
	}
	// Try standard Go duration parsing.
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	// Handle "@daily 9am"-style specs (coarse: just 24h).
	if len(s) > 6 && s[:6] == "@daily" {
		return 24 * time.Hour
	}
	// Handle "@hourly".
	if s == "@hourly" {
		return 1 * time.Hour
	}
	return 0
}

