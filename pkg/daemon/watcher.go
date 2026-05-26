package daemon

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileEvent represents a filesystem change detected by the watcher.
type FileEvent struct {
	Path      string    `json:"path"`
	Event     string    `json:"event"` // write, create, delete
	Timestamp time.Time `json:"timestamp"`
}

// WatchRule defines a pattern to watch and the action to trigger.
type WatchRule struct {
	Name      string        `json:"name"`
	Path      string        `json:"path"` // glob pattern, e.g. "*.go"
	Events    []string      `json:"events"`
	Trigger   string        `json:"trigger"` // agent type to spawn
	Ignore    []string      `json:"ignore"`
	Cooldown  time.Duration `json:"cooldown"`
	lastFired time.Time
}

// FileWatcher polls the filesystem for changes and triggers agent actions.
type FileWatcher struct {
	mu       sync.RWMutex
	rules    []WatchRule
	interval time.Duration
	snapshots map[string]time.Time // path -> last seen mtime
}

// NewFileWatcher creates a watcher with the given poll interval.
func NewFileWatcher(interval time.Duration) *FileWatcher {
	w := &FileWatcher{
		interval:  interval,
		snapshots: make(map[string]time.Time),
	}
	w.loadDefaults()
	return w
}

// loadDefaults seeds the watcher with sensible default rules.
func (w *FileWatcher) loadDefaults() {
	w.rules = []WatchRule{
		{Name: "go-file-write", Path: "*.go", Events: []string{"write"}, Trigger: "code-reviewer", Cooldown: 5 * time.Minute},
		{Name: "go-mod-write", Path: "go.{mod,sum}", Events: []string{"write"}, Trigger: "dependency-audit", Cooldown: 1 * time.Minute},
		{Name: "go-file-create", Path: "*.go", Events: []string{"create"}, Trigger: "update-context", Cooldown: 10 * time.Minute},
	}
}

// AddRule registers a new watch rule.
func (w *FileWatcher) AddRule(rule WatchRule) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rules = append(w.rules, rule)
}

// RemoveRule removes a rule by name.
func (w *FileWatcher) RemoveRule(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, r := range w.rules {
		if r.Name == name {
			w.rules = append(w.rules[:i], w.rules[i+1:]...)
			return
		}
	}
}

// Start begins the poll loop, checking for file changes every interval.
// Emitted events are logged and would trigger agent spawns via the kernel.
func (w *FileWatcher) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

// poll checks tracked paths for mtime changes and emits events.
func (w *FileWatcher) poll() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i := range w.rules {
		rule := &w.rules[i]
		matches, _ := filepath.Glob(rule.Path)
		for _, path := range matches {
			if w.isIgnored(rule, path) {
				continue
			}
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			lastMtime, known := w.snapshots[path]
			currentMtime := info.ModTime()

			if !known {
				// New file detected.
				w.snapshots[path] = currentMtime
				if rule.hasEvent("create") {
					w.fire(rule, path, "create")
				}
			} else if currentMtime.After(lastMtime) {
				w.snapshots[path] = currentMtime
				if rule.hasEvent("write") {
					w.fire(rule, path, "write")
				}
			}
		}
	}
}

// fire emits a file event (logs it; in production would spawn an agent).
func (w *FileWatcher) fire(rule *WatchRule, path, event string) {
	now := time.Now()
	if now.Sub(rule.lastFired) < rule.Cooldown {
		return
	}
	rule.lastFired = now
	// In production this would call kernel.Spawn(rule.Trigger, ...).
	_ = FileEvent{Path: path, Event: event, Timestamp: now}
}

// isIgnored checks if a path matches any ignore patterns.
func (w *FileWatcher) isIgnored(rule *WatchRule, path string) bool {
	for _, pattern := range rule.Ignore {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}
	}
	return false
}

// hasEvent checks whether the rule tracks the given event type.
func (r *WatchRule) hasEvent(event string) bool {
	for _, e := range r.Events {
		if e == event {
			return true
		}
	}
	return false
}
