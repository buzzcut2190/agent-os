package team

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// CreateTask inserts a task into the store, generating a UUIDv4 ID when
// empty. Rejects empty titles and dependency cycles.
func (s *TeamStore) CreateTask(t Task) error {
	if t.Title == "" {
		return errors.New("task title cannot be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if t.ID == "" {
		t.ID = newUUID()
	}
	if _, exists := s.tasks[t.ID]; exists {
		return fmt.Errorf("task with id %s already exists", t.ID)
	}
	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = TaskCreated
	}
	if len(t.DependsOn) > 0 {
		if err := s.detectCycleLocked(t.ID, t.DependsOn); err != nil {
			return err
		}
	}
	s.tasks[t.ID] = &t
	return nil
}

// CreateTaskFromYAML parses a YAML-encoded task and stores it.
func (s *TeamStore) CreateTaskFromYAML(data []byte) (*Task, error) {
	var t Task
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("failed to parse task YAML: %w", err)
	}
	if t.ID == "" {
		t.ID = newUUID()
	}
	if err := s.CreateTask(t); err != nil {
		return nil, err
	}
	task, _ := s.GetTask(t.ID)
	return task, nil
}

// GetTask returns a copy of the task identified by id.
func (s *TeamStore) GetTask(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tasks[id]
	if !ok {
		return nil, false
	}
	cp := *t
	cp.DependsOn = copySlice(t.DependsOn)
	return &cp, true
}

// UpdateTaskStatus transitions a task after validating the transition.
// When unblocking (blocked->in_progress) it verifies dependencies are done.
func (s *TeamStore) UpdateTaskStatus(id string, status TaskStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if !ValidTransition(task.Status, status) {
		return fmt.Errorf("invalid status transition: %s -> %s", task.Status, status)
	}
	if task.Status == TaskBlocked && status == TaskInProgress {
		for _, depID := range task.DependsOn {
			dep, ok := s.tasks[depID]
			if !ok || dep.Status != TaskDone {
				return fmt.Errorf("cannot unblock task %s: dependency %s is not done", id, depID)
			}
		}
	}
	task.Status = status
	task.UpdatedAt = time.Now()
	return nil
}

// AssignTask sets the assignee and transitions the task to TaskAssigned.
func (s *TeamStore) AssignTask(id, agentName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if _, ok := s.agents[agentName]; !ok {
		return fmt.Errorf("agent %s not registered", agentName)
	}
	task.Assignee = agentName
	task.Status = TaskAssigned
	task.UpdatedAt = time.Now()
	return nil
}

// ListTasksByStatus returns copies of tasks matching the given status.
func (s *TeamStore) ListTasksByStatus(status TaskStatus) []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Task
	for _, t := range s.tasks {
		if t.Status == status {
			cp := *t
			cp.DependsOn = copySlice(t.DependsOn)
			result = append(result, &cp)
		}
	}
	return result
}

// BoardText generates a 4-column ASCII kanban board.
func (s *TeamStore) BoardText() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var todo, prog, review, done []*Task
	var blocked []*Task
	for _, t := range s.tasks {
		switch t.Status {
		case TaskCreated:
			todo = append(todo, t)
		case TaskAssigned, TaskInProgress:
			prog = append(prog, t)
		case TaskReview:
			review = append(review, t)
		case TaskDone:
			done = append(done, t)
		default:
			blocked = append(blocked, t)
		}
	}
	cols := [4][]*Task{todo, prog, review, done}
	headers := [4]string{"[TODO]", "[IN PROGRESS]", "[REVIEW]", "[DONE]"}
	colW := 30
	var sb strings.Builder
	sb.WriteString("=== KANBAN BOARD ===\n\n")
	for _, h := range headers {
		sb.WriteString(fmt.Sprintf("%-*s", colW, h))
	}
	sb.WriteString("\n")
	for range 4 {
		sb.WriteString(strings.Repeat("-", colW))
	}
	sb.WriteString("\n")
	maxRows := 0
	for _, c := range cols {
		if len(c) > maxRows {
			maxRows = len(c)
		}
	}
	for row := 0; row < maxRows; row++ {
		for c := 0; c < 4; c++ {
			if row < len(cols[c]) {
				task := cols[c][row]
				text := fmt.Sprintf("%s [%s]", task.Title, task.Assignee)
				if len(text) > colW-2 {
					text = text[:colW-5] + "..."
				}
				sb.WriteString(fmt.Sprintf("%-*s", colW, text))
			} else {
				sb.WriteString(fmt.Sprintf("%-*s", colW, ""))
			}
		}
		sb.WriteString("\n")
	}
	if len(blocked) > 0 {
		sb.WriteString("\n--- Blocked / Rejected ---\n")
		for _, t := range blocked {
			sb.WriteString(fmt.Sprintf("  [%s] %s  (assignee: %s)\n", t.Status, t.Title, t.Assignee))
		}
	}
	return []byte(sb.String()), nil
}

// DetectCycle checks whether adding a task with the given dependencies
// would introduce a cycle in the dependency graph.
func (s *TeamStore) DetectCycle(newTaskID string, dependsOn []string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.detectCycleLocked(newTaskID, dependsOn)
}

// detectCycleLocked performs DFS cycle detection. Caller holds the lock.
func (s *TeamStore) detectCycleLocked(newTaskID string, dependsOn []string) error {
	state := make(map[string]int) // 0=unvisited, 1=visiting, 2=done
	var dfs func(id string) bool
	dfs = func(id string) bool {
		st := state[id]
		if st == 1 {
			return true
		}
		if st == 2 {
			return false
		}
		state[id] = 1
		if t, ok := s.tasks[id]; ok {
			for _, dep := range t.DependsOn {
				if dfs(dep) {
					return true
				}
			}
		}
		state[id] = 2
		return false
	}

	placeholder := &Task{ID: newTaskID, DependsOn: dependsOn}
	existing, existed := s.tasks[newTaskID]
	s.tasks[newTaskID] = placeholder

	cycle := dfs(newTaskID)
	for id := range s.tasks {
		if state[id] == 0 && dfs(id) {
			cycle = true
			break
		}
	}

	if existed {
		s.tasks[newTaskID] = existing
	} else {
		delete(s.tasks, newTaskID)
	}
	if cycle {
		return errors.New("cycle detected in task dependencies")
	}
	return nil
}

// TopologicalOrder returns tasks in dependency order (Kahn's algorithm).
func (s *TeamStore) TopologicalOrder() ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inDegree := make(map[string]int)
	adj := make(map[string][]string)

	for id := range s.tasks {
		inDegree[id] = 0
	}
	for id, t := range s.tasks {
		for _, dep := range t.DependsOn {
			if _, ok := s.tasks[dep]; ok {
				adj[dep] = append(adj[dep], id)
				inDegree[id]++
			}
		}
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

	result := make([]*Task, 0, len(s.tasks))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		t := s.tasks[id]
		cp := *t
		cp.DependsOn = copySlice(t.DependsOn)
		result = append(result, &cp)
		for _, next := range adj[id] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if len(result) != len(s.tasks) {
		return nil, errors.New("cycle detected in task dependency graph")
	}
	return result, nil
}
