package team

import (
	"crypto/rand"
	"fmt"
	"time"
)

// AgentRole classifies an agent's function within the team.
type AgentRole string

const (
	RoleArchitect   AgentRole = "architect"
	RoleDeveloper   AgentRole = "developer"
	RoleTester      AgentRole = "tester"
	RoleReviewer    AgentRole = "reviewer"
	RoleCoordinator AgentRole = "coordinator"
)

// AgentStatus indicates whether an agent is available.
type AgentStatus string

const (
	AgentOnline  AgentStatus = "online"
	AgentOffline AgentStatus = "offline"
	AgentBusy    AgentStatus = "busy"
)

// AgentInfo describes a registered team member.
type AgentInfo struct {
	ID         string      `yaml:"id"`
	Name       string      `yaml:"name"`
	Role       AgentRole   `yaml:"role"`
	Status     AgentStatus `yaml:"status"`
	Workspace  string      `yaml:"workspace"`
	Registered time.Time   `yaml:"registered"`
}

// TaskStatus represents the lifecycle stage of a task.
type TaskStatus string

const (
	TaskCreated    TaskStatus = "created"
	TaskAssigned   TaskStatus = "assigned"
	TaskInProgress TaskStatus = "in_progress"
	TaskReview     TaskStatus = "review"
	TaskDone       TaskStatus = "done"
	TaskRejected   TaskStatus = "rejected"
	TaskBlocked    TaskStatus = "blocked"
)

// Task is a unit of work assigned to an agent.
type Task struct {
	ID          string     `yaml:"id"`
	Title       string     `yaml:"title"`
	Description string     `yaml:"description"`
	Assignee    string     `yaml:"assignee"`
	Status      TaskStatus `yaml:"status"`
	DependsOn   []string   `yaml:"depends_on,omitempty"`
	Output      string     `yaml:"output,omitempty"`
	CreatedAt   time.Time  `yaml:"created_at"`
	UpdatedAt   time.Time  `yaml:"updated_at"`
}

// Message represents a message sent between agents.
type Message struct {
	ID          string    `yaml:"id"`
	From        string    `yaml:"from"`
	To          string    `yaml:"to"`
	Subject     string    `yaml:"subject"`
	Body        string    `yaml:"body"`
	Attachments []string  `yaml:"attachments,omitempty"`
	ThreadID    string    `yaml:"thread_id,omitempty"`
	Timestamp   time.Time `yaml:"timestamp"`
	Read        bool      `yaml:"read"`
}

// SharedContext is a shared knowledge entry accessible to multiple agents.
type SharedContext struct {
	ID       string            `yaml:"id"`
	Authors  []string          `yaml:"authors"`
	Tags     []string          `yaml:"tags"`
	Content  string            `yaml:"content"`
	Metadata map[string]string `yaml:"metadata,omitempty"`
	Expiry   time.Time         `yaml:"expiry"`
	Created  time.Time         `yaml:"created"`
}

// TopologyType describes how agents are organized.
type TopologyType string

const (
	TopoPipeline  TopologyType = "pipeline"
	TopoStar      TopologyType = "star"
	TopoMesh      TopologyType = "mesh"
	TopoHierarchy TopologyType = "hierarchy"
)

// Topology defines the team's communication and scheduling structure.
type Topology struct {
	Type    TopologyType       `yaml:"type"`
	Agents  []string           `yaml:"agents"`
	Options map[string]string  `yaml:"options,omitempty"`
}

// validTransitions defines the allowed task-status state machine.
var validTransitions = map[TaskStatus]map[TaskStatus]bool{
	TaskCreated:    {TaskAssigned: true, TaskBlocked: true},
	TaskAssigned:   {TaskInProgress: true, TaskRejected: true},
	TaskInProgress: {TaskReview: true, TaskBlocked: true},
	TaskReview:     {TaskDone: true, TaskRejected: true},
	TaskRejected:   {TaskCreated: true},
	TaskBlocked:    {TaskInProgress: true, TaskCreated: true},
}

// ValidTransition reports whether transitioning from one TaskStatus to
// another is permitted. Identity transitions (from == to) are always valid.
func ValidTransition(from, to TaskStatus) bool {
	if from == to {
		return true
	}
	tos, ok := validTransitions[from]
	if !ok {
		return false
	}
	return tos[to]
}

// copySlice returns a copy of a string slice.
func copySlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

// copyMap returns a shallow copy of a string map.
func copyMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// newUUID generates a version-4 UUID using crypto/rand.
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
