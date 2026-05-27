package sandbox

import (
	"time"
)

// Session represents an isolated workspace session using file-system copies.
type Session struct {
	ID        string        `json:"id"`
	Created   time.Time     `json:"created"`
	Project   string        `json:"project"`    // original project root (read-only reference)
	Workspace string        `json:"workspace"`  // working copy of the project
	Status    SessionStatus `json:"status"`
}

// SessionStatus represents the current state of a session.
type SessionStatus string

const (
	StatusActive    SessionStatus = "active"
	StatusCommitted SessionStatus = "committed"
	StatusDiscarded SessionStatus = "discarded"
)

// Manager handles all session lifecycle operations.
type Manager struct {
	BaseDir string // ~/.local/share/agentfs/sessions/
}

// DiffEntry represents a single file change between workspace and project.
type DiffEntry struct {
	Path   string `json:"path"`
	Status string `json:"status"` // "added", "modified", "deleted"
}
