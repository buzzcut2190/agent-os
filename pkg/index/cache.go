package index

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AnalysisCache wraps an Engine with modification-aware caching so that
// analysis is only re-run when tracked source files have changed.
type AnalysisCache struct {
	mu      sync.RWMutex
	engine  *Engine
	modTime map[string]time.Time // relative path -> last known mtime
}

// NewAnalysisCache creates a cache backed by the given engine.
func NewAnalysisCache(engine *Engine) *AnalysisCache {
	return &AnalysisCache{
		engine:  engine,
		modTime: make(map[string]time.Time),
	}
}

// Get returns the current analysis, re-running only if tracked files
// were modified since the last call.
func (c *AnalysisCache) Get() (*AnalysisResult, error) {
	changed := c.checkModTimes()
	if len(changed) == 0 {
		c.mu.RLock()
		cached := c.engine.cached
		c.mu.RUnlock()
		if cached != nil {
			return cached, nil
		}
	}

	c.engine.Invalidate()
	result, err := c.engine.GetAnalysis()
	if err != nil {
		return nil, err
	}
	c.updateModTimes(result)
	return result, nil
}

// checkModTimes returns the relative paths of tracked files whose on-disk
// modification time differs from the last known value.
func (c *AnalysisCache) checkModTimes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var changed []string
	for rel, prev := range c.modTime {
		abs := filepath.Join(c.engine.RootDir, rel)
		info, err := os.Stat(abs)
		if err != nil {
			// File was deleted — treat as changed.
			changed = append(changed, rel)
			continue
		}
		if !info.ModTime().Equal(prev) {
			changed = append(changed, rel)
		}
	}
	return changed
}

// updateModTimes scans every file referenced by the analysis result and
// records its current on-disk modification time.
func (c *AnalysisCache) updateModTimes(r *AnalysisResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	seen := make(map[string]bool)
	for _, s := range r.Symbols {
		seen[s.Def.File] = true
		for _, ref := range s.Refs {
			seen[ref.File] = true
		}
	}

	for rel := range seen {
		abs := filepath.Join(c.engine.RootDir, rel)
		info, err := os.Stat(abs)
		if err != nil {
			continue
		}
		c.modTime[rel] = info.ModTime()
	}
}

// Invalidate clears all cached mod-time records and tells the underlying
// engine to regenerate on the next Get call.
func (c *AnalysisCache) Invalidate() {
	c.mu.Lock()
	c.modTime = make(map[string]time.Time)
	c.mu.Unlock()
	c.engine.Invalidate()
}
