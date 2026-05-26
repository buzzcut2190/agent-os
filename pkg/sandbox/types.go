package sandbox

import (
	"time"
)

// Session represents an isolated workspace overlay session.
type Session struct {
	ID      string        `json:"id"`
	Created time.Time     `json:"created"`
	Project string        `json:"project"` // original project root
	Lower   string        `json:"lower"`   // lower layer (read-only original)
	Upper   string        `json:"upper"`   // upper layer (writable copy-on-write)
	Work    string        `json:"work"`    // overlay work dir
	Merged  string        `json:"merged"`  // overlay mount point
	Status  SessionStatus `json:"status"`
}

// SessionStatus represents the current state of a session.
type SessionStatus string

const (
	StatusActive    SessionStatus = "active"
	StatusCommitted SessionStatus = "committed"
	StatusDiscarded SessionStatus = "discarded"
)

// Overlay manages an OverlayFS mount for session isolation.
type Overlay struct {
	Lower  string
	Upper  string
	Work   string
	Merged string
}

// Manager handles all session lifecycle operations.
type Manager struct {
	BaseDir string // ~/.local/share/agentfs/sessions/
}
