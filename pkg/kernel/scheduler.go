package kernel

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Scheduler dispatches tasks to agents based on priority and dependencies.
type Scheduler struct {
	mu       sync.Mutex
	tasks    map[TaskID]*Task
	pending  []*Task // priority-ordered
	lm       *LifecycleManager
}

// NewScheduler creates a task scheduler.
func NewScheduler(lm *LifecycleManager) *Scheduler {
	return &Scheduler{
		tasks: make(map[TaskID]*Task),
		lm:    lm,
	}
}

// Submit adds a task to the scheduling queue.
func (s *Scheduler) Submit(task Task) (TaskID, error) {
	if task.ID == "" {
		task.ID = TaskID(uuid.New().String()[:8])
	}
	if task.Priority < 0 || task.Priority > 100 {
		return "", fmt.Errorf("priority must be 0-100")
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	task.State = TaskPending
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = &task
	s.insertPending(&task)
	return task.ID, nil
}

// Cancel marks a task as cancelled.
func (s *Scheduler) Cancel(id TaskID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if t.State == TaskRunning || t.State == TaskCompleted {
		return fmt.Errorf("cannot cancel task in state %s", t.State)
	}
	t.State = TaskCancelled
	// Remove from pending slice.
	for i, pt := range s.pending {
		if pt.ID == id {
			s.pending = append(s.pending[:i], s.pending[i+1:]...)
			break
		}
	}
	return nil
}

// ListPending returns all pending tasks ordered by priority.
func (s *Scheduler) ListPending() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Task, 0, len(s.pending))
	for _, t := range s.pending {
		if t.State == TaskPending {
			result = append(result, *t)
		}
	}
	return result
}

// List returns tasks optionally filtered by state.
func (s *Scheduler) List(filter TaskState) []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []Task
	for _, t := range s.tasks {
		if filter == "" || t.State == filter {
			result = append(result, *t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})
	return result
}

// Dispatch returns the next ready task and an available agent.
func (s *Scheduler) Dispatch() (*Task, *Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, t := range s.pending {
		if !s.depsSatisfied(t) {
			continue
		}
		t.State = TaskScheduled
		// Find or spawn an agent of the required type.
		agents := s.lm.List(AgentCreated)
		for _, a := range agents {
			if a.Type == t.AgentType {
				s.lm.Run(a.ID)
				t.State = TaskRunning
				t.AssignedTo = a.ID
				return t, a, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("no ready task available")
}

// Start begins the scheduling loop.
func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _, _ = s.Dispatch()
			}
		}
	}()
}

func (s *Scheduler) insertPending(t *Task) {
	s.pending = append(s.pending, t)
	sort.Slice(s.pending, func(i, j int) bool {
		return s.pending[i].Priority > s.pending[j].Priority
	})
}

func (s *Scheduler) depsSatisfied(t *Task) bool {
	for _, depID := range t.Dependencies {
		dep, ok := s.tasks[depID]
		if !ok || dep.State != TaskCompleted {
			return false
		}
	}
	return true
}
