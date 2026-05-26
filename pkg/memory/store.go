package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store provides persistent memory storage using JSONL files.
type Store struct {
	mu      sync.RWMutex
	baseDir string
	storePath string
	sessionsDir string
	decisionsDir string
	prefsDir string
	knowledgeDir string
	ttlDays int
}

// NewStore creates a memory store rooted at baseDir.
func NewStore(baseDir string, ttlDays int) (*Store, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}
	s := &Store{
		baseDir:      baseDir,
		storePath:    filepath.Join(baseDir, "store.jsonl"),
		sessionsDir:  filepath.Join(baseDir, "sessions"),
		decisionsDir: filepath.Join(baseDir, "decisions"),
		prefsDir:     filepath.Join(baseDir, "preferences"),
		knowledgeDir: filepath.Join(baseDir, "knowledge"),
		ttlDays:      ttlDays,
	}
	for _, d := range []string{s.sessionsDir, s.decisionsDir, s.prefsDir, s.knowledgeDir} {
		os.MkdirAll(d, 0755)
	}
	return s, nil
}

// Save persists a memory entry.
func (s *Store) Save(m Memory) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.append(m)
}

// Get retrieves a memory by ID.
func (s *Store) Get(id string) (Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	memories, err := s.readAll()
	if err != nil {
		return Memory{}, err
	}
	for _, m := range memories {
		if m.ID == id {
			return m, nil
		}
	}
	return Memory{}, fmt.Errorf("memory %s not found", id)
}

// Search finds memories matching the given options.
func (s *Store) Search(opts SearchOpts) ([]Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	memories, err := s.readAll()
	if err != nil {
		return nil, err
	}
	var results []Memory
	for _, m := range memories {
		if !s.matches(m, opts) {
			continue
		}
		results = append(results, m)
	}
	// Newest first.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return results, nil
}

// List returns recent memories, optionally filtered by type.
func (s *Store) List(typeFilter MemType, limit int) ([]Memory, error) {
	opts := SearchOpts{Limit: limit}
	if typeFilter != "" {
		opts.Types = []MemType{typeFilter}
	}
	return s.Search(opts)
}

// Delete removes a memory by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.filterAndSave(func(m Memory) bool { return m.ID != id })
}

// Forget removes memories older than the given time, optionally filtered by type.
func (s *Store) Forget(before time.Time, types ...MemType) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	err := s.filterAndSave(func(m Memory) bool {
		if m.Timestamp.After(before) {
			return true
		}
		if len(types) > 0 {
			for _, t := range types {
				if m.Type == t {
					count++
					return false
				}
			}
			return true
		}
		count++
		return false
	})
	return count, err
}

// Stats returns aggregate statistics about stored memories.
func (s *Store) Stats() (Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	memories, err := s.readAll()
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{
		ByType: make(map[MemType]int),
	}
	for _, m := range memories {
		stats.Total++
		stats.ByType[m.Type]++
		if stats.Earliest.IsZero() || m.Timestamp.Before(stats.Earliest) {
			stats.Earliest = m.Timestamp
		}
		if m.Timestamp.After(stats.Latest) {
			stats.Latest = m.Timestamp
		}
	}
	return stats, nil
}

// Recent returns the last n memories.
func (s *Store) Recent(n int) ([]Memory, error) {
	return s.List("", n)
}

// matches checks if a memory matches the search options.
func (s *Store) matches(m Memory, opts SearchOpts) bool {
	if len(opts.Types) > 0 {
		found := false
		for _, t := range opts.Types {
			if m.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(opts.Tags) > 0 {
		found := false
		for _, t := range opts.Tags {
			for _, mt := range m.Tags {
				if mt == t {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}
	if opts.Source != "" && m.Source != opts.Source {
		return false
	}
	if !opts.Since.IsZero() && m.Timestamp.Before(opts.Since) {
		return false
	}
	if !opts.Before.IsZero() && m.Timestamp.After(opts.Before) {
		return false
	}
	if opts.Query != "" {
		query := strings.ToLower(opts.Query)
		if !strings.Contains(strings.ToLower(m.Content), query) &&
			!s.tagContains(m.Tags, query) {
			return false
		}
	}
	return true
}

func (s *Store) tagContains(tags []string, query string) bool {
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), query) {
			return true
		}
	}
	return false
}

// append writes a single memory entry to the JSONL file.
func (s *Store) append(m Memory) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.storePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, string(data))
	return err
}

// readAll reads all memories from the JSONL file.
func (s *Store) readAll() ([]Memory, error) {
	f, err := os.Open(s.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var memories []Memory
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var m Memory
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			continue
		}
		memories = append(memories, m)
	}
	return memories, scanner.Err()
}

// filterAndSave rewrites the store, keeping only memories that pass the filter.
func (s *Store) filterAndSave(keep func(Memory) bool) error {
	memories, err := s.readAll()
	if err != nil {
		return err
	}
	var kept []Memory
	for _, m := range memories {
		if keep(m) {
			kept = append(kept, m)
		}
	}
	tmpPath := s.storePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, m := range kept {
		data, _ := json.Marshal(m)
		fmt.Fprintln(w, string(data))
	}
	w.Flush()
	f.Close()
	return os.Rename(tmpPath, s.storePath)
}
