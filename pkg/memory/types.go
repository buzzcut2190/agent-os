package memory

import "time"

// MemType classifies a memory entry.
type MemType string

const (
	MemSession    MemType = "session"
	MemDecision   MemType = "decision"
	MemPreference MemType = "preference"
	MemKnowledge  MemType = "knowledge"
)

// Memory represents a single persistent memory entry.
type Memory struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Type      MemType           `json:"type"`
	Source    string            `json:"source"`
	Content   string            `json:"content"`
	Tags      []string          `json:"tags"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SearchOpts controls memory retrieval.
type SearchOpts struct {
	Types  []MemType `json:"types,omitempty"`
	Tags   []string  `json:"tags,omitempty"`
	Source string    `json:"source,omitempty"`
	Since  time.Time `json:"since,omitempty"`
	Before time.Time `json:"before,omitempty"`
	Limit  int       `json:"limit,omitempty"`
	Query  string    `json:"query,omitempty"` // full-text search
}

// MemInfo is a summary returned by List/Search.
type MemInfo struct {
	ID        string   `json:"id"`
	Timestamp string   `json:"timestamp"`
	Type      MemType  `json:"type"`
	Source    string   `json:"source"`
	Tags      []string `json:"tags"`
	Snippet   string   `json:"snippet"` // first 200 chars
}

// Stats holds aggregate memory statistics.
type Stats struct {
	Total     int            `json:"total"`
	ByType    map[MemType]int `json:"by_type"`
	Earliest  time.Time      `json:"earliest"`
	Latest    time.Time      `json:"latest"`
}
