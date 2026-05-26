package kernel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// StateStore persists agent state to a JSONL file.
type StateStore struct {
	path string
}

// NewStateStore creates a state store at the given path.
func NewStateStore(path string) *StateStore {
	os.MkdirAll(filepath.Dir(path), 0755)
	return &StateStore{path: path}
}

// Save writes all agent states to the JSONL file.
func (s *StateStore) Save(agents []*Agent) error {
	f, err := os.Create(s.path)
	if err != nil {
		return fmt.Errorf("create state file: %w", err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, a := range agents {
		data, _ := json.Marshal(a)
		fmt.Fprintln(w, string(data))
	}
	return w.Flush()
}

// Load reads all agent states from the JSONL file.
func (s *StateStore) Load() ([]*Agent, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var agents []*Agent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var a Agent
		if err := json.Unmarshal(scanner.Bytes(), &a); err != nil {
			continue
		}
		agents = append(agents, &a)
	}
	return agents, scanner.Err()
}

// Restore loads saved agents back into a LifecycleManager.
func (s *StateStore) Restore(lm *LifecycleManager) error {
	agents, err := s.Load()
	if err != nil {
		return err
	}
	lm.mu.Lock()
	defer lm.mu.Unlock()
	for _, a := range agents {
		lm.agents[a.ID] = a
	}
	return nil
}

// Snapshot saves current agent state from a LifecycleManager.
func (s *StateStore) Snapshot(lm *LifecycleManager) error {
	agents := lm.List("")
	return s.Save(agents)
}
