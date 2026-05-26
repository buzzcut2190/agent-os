package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AgentMemory provides persistent learning storage and session logging
// for an agent workspace.
type AgentMemory struct {
	root string
}

// NewAgentMemory creates an AgentMemory rooted at the given path
// (typically <workspace>/memory/).
func NewAgentMemory(root string) *AgentMemory {
	return &AgentMemory{root: root}
}

// learningsPath returns the path to memory/learnings.md.
func (m *AgentMemory) learningsPath() string {
	return filepath.Join(m.root, "learnings.md")
}

// sessionsDir returns the path to memory/sessions/.
func (m *AgentMemory) sessionsDir() string {
	return filepath.Join(m.root, "sessions")
}

// Learn appends a topic and content entry to the learnings markdown
// file. The entry is timestamped.
func (m *AgentMemory) Learn(topic, content string) error {
	f, err := os.OpenFile(m.learningsPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("memory: learn: %w", err)
	}
	defer f.Close()

	entry := fmt.Sprintf(
		"\n## %s\n*%s*\n\n%s\n",
		topic,
		time.Now().Format(time.RFC3339),
		content,
	)
	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("memory: learn write: %w", err)
	}
	return nil
}

// Recall performs a simple case‑insensitive substring search across all
// learning entries and returns the matching topic sections. When query
// is empty all learnings are returned.
func (m *AgentMemory) Recall(query string) (string, error) {
	data, err := os.ReadFile(m.learningsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("memory: recall: %w", err)
	}
	if len(data) == 0 {
		return "", nil
	}

	content := string(data)
	if query == "" {
		return content, nil
	}

	// Split on "## " headings and match against the query.
	lowerQuery := strings.ToLower(query)
	sections := strings.Split(content, "\n## ")
	var matched []string

	for _, sec := range sections {
		if sec == "" {
			continue
		}
		if strings.Contains(strings.ToLower(sec), lowerQuery) {
			entry := sec
			// Re-add "## " prefix unless this is the very first section
			// which was split differently.
			if sec != sections[0] || !strings.HasPrefix(content, "## ") {
				entry = "## " + sec
			}
			matched = append(matched, entry)
		}
	}

	return strings.Join(matched, "\n"), nil
}

// SessionEntry represents a single logged session.
type SessionEntry struct {
	Filename  string
	Content   string
	Timestamp time.Time
}

// LogSession writes a session entry to a timestamped file under
// memory/sessions/.
func (m *AgentMemory) LogSession(content string) error {
	if err := os.MkdirAll(m.sessionsDir(), 0o755); err != nil {
		return fmt.Errorf("memory: log session: %w", err)
	}

	now := time.Now()
	filename := now.Format("2006-01-02T150405") + ".md"
	path := filepath.Join(m.sessionsDir(), filename)

	entry := fmt.Sprintf(
		"# Session %s\n\n%s\n",
		now.Format(time.RFC3339),
		content,
	)
	if err := os.WriteFile(path, []byte(entry), 0o644); err != nil {
		return fmt.Errorf("memory: log session write: %w", err)
	}
	return nil
}

// RecentSessions returns up to n most‑recent session entries sorted by
// timestamp descending.
func (m *AgentMemory) RecentSessions(n int) ([]SessionEntry, error) {
	entries, err := os.ReadDir(m.sessionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("memory: recent sessions: %w", err)
	}

	var sessions []SessionEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(m.sessionsDir(), e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		sessions = append(sessions, SessionEntry{
			Filename:  e.Name(),
			Content:   string(data),
			Timestamp: info.ModTime(),
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Timestamp.After(sessions[j].Timestamp)
	})

	if n > 0 && n < len(sessions) {
		sessions = sessions[:n]
	}
	return sessions, nil
}
