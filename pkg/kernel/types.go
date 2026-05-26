package kernel

import "time"

// AgentID uniquely identifies an agent instance.
type AgentID string

// TaskID uniquely identifies a schedulable task.
type TaskID string

// AgentType classifies the kind of work an agent is designed for.
type AgentType string

const (
	AgentTypeCodeReview   AgentType = "code-reviewer"
	AgentTypeArchitect    AgentType = "architect"
	AgentTypeDeveloper    AgentType = "developer"
	AgentTypeTester       AgentType = "tester"
	AgentTypeMonitor      AgentType = "monitor"
	AgentTypeReporter     AgentType = "reporter"
	AgentTypeOrchestrator AgentType = "orchestrator"
	AgentTypeUserDefined  AgentType = "user-defined"
)

// AgentState represents the current lifecycle stage of an agent.
type AgentState string

const (
	AgentCreated    AgentState = "created"
	AgentRunning    AgentState = "running"
	AgentSuspended  AgentState = "suspended"
	AgentBlocked    AgentState = "blocked"
	AgentTerminated AgentState = "terminated"
	AgentFailed     AgentState = "failed"
)

// Agent is the complete representation of an agent instance.
type Agent struct {
	ID        AgentID       `json:"id"`
	Type      AgentType     `json:"type"`
	State     AgentState    `json:"state"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Config    AgentConfig   `json:"config"`
	Resources ResourceUsage `json:"resources"`
	Log       []LogEntry    `json:"log,omitempty"`
	ParentID  AgentID       `json:"parent_id,omitempty"`
	Children  []AgentID     `json:"children,omitempty"`
}

// AgentConfig holds the spawn-time configuration for an agent.
type AgentConfig struct {
	Model        string            `json:"model"`
	Provider     string            `json:"provider"`
	Skills       []string          `json:"skills"`
	MaxTokens    int               `json:"max_tokens"`
	Timeout      time.Duration     `json:"timeout"`
	Labels       map[string]string `json:"labels"`
	SystemPrompt string            `json:"system_prompt,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
}

// ResourceUsage tracks token and compute consumption for one agent.
type ResourceUsage struct {
	TokensUsed    int     `json:"tokens_used"`
	TokensLimit   int     `json:"tokens_limit"`
	CPUPercent    float64 `json:"cpu_percent,omitempty"`
	MemoryBytes   int64   `json:"memory_bytes,omitempty"`
	UptimeSeconds int64   `json:"uptime_seconds,omitempty"`
}

// LogEntry is a single event logged by an agent.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

// TaskState is the lifecycle stage of a task.
type TaskState string

const (
	TaskPending   TaskState = "pending"
	TaskScheduled TaskState = "scheduled"
	TaskRunning   TaskState = "running"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
	TaskCancelled TaskState = "cancelled"
)

// Task is a schedulable unit of work.
type Task struct {
	ID           TaskID           `json:"id"`
	Name         string            `json:"name"`
	Priority     int               `json:"priority"`
	Dependencies []TaskID          `json:"dependencies"`
	AgentType    AgentType         `json:"agent_type"`
	Payload      map[string]any    `json:"payload"`
	CreatedAt    time.Time         `json:"created_at"`
	State        TaskState         `json:"state"`
	AssignedTo   AgentID           `json:"assigned_to,omitempty"`
}

// ResourceLimits defines global resource caps.
type ResourceLimits struct {
	MaxAgents         int `json:"max_agents"`
	MaxTokensPerAgent int `json:"max_tokens_per_agent"`
	MaxTotalTokens    int `json:"max_total_tokens"`
	MaxAgentsPerType  int `json:"max_agents_per_type"`
}

// DefaultResourceLimits returns sensible defaults.
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{
		MaxAgents:         50,
		MaxTokensPerAgent: 200000,
		MaxTotalTokens:    2000000,
		MaxAgentsPerType:  10,
	}
}

// ResourceSummary describes global resource usage.
type ResourceSummary struct {
	ActiveAgents int             `json:"active_agents"`
	TotalAgents  int             `json:"total_agents"`
	TokensUsed   int             `json:"tokens_used"`
	TokensLimit  int             `json:"tokens_limit"`
	ByType       map[AgentType]int `json:"by_type"`
}
