package team

import (
	"fmt"
	"strings"
	"time"
)

// Orchestrator provides high-level team management: decomposing goals into
// tasks, scheduling assignments, reassigning work, and monitoring progress.
type Orchestrator struct {
	store *TeamStore
}

// NewOrchestrator creates an Orchestrator backed by the given store.
func NewOrchestrator(store *TeamStore) *Orchestrator {
	return &Orchestrator{store: store}
}

// DecomposeGoal breaks a high-level goal string into concrete tasks using
// keyword-based templates. Returns tasks with pre-generated IDs and
// dependency edges.
func (o *Orchestrator) DecomposeGoal(goal string) ([]Task, error) {
	lower := strings.ToLower(goal)
	now := time.Now()

	switch {
	case strings.Contains(lower, "implement") && strings.Contains(lower, "login"):
		ids := genIDs(5)
		return []Task{
			{ID: ids[0], Title: "Design auth flow", Description: "Design the authentication flow and data model", CreatedAt: now, UpdatedAt: now},
			{ID: ids[1], Title: "Implement login endpoint", Description: "Implement the login API endpoint", DependsOn: []string{ids[0]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[2], Title: "Add password hashing", Description: "Implement secure password hashing", DependsOn: []string{ids[0]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[3], Title: "Write auth tests", Description: "Write unit and integration tests for auth", DependsOn: []string{ids[1], ids[2]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[4], Title: "Review and finalize", Description: "Review the implementation and merge", DependsOn: []string{ids[3]}, CreatedAt: now, UpdatedAt: now},
		}, nil

	case strings.Contains(lower, "add") && (strings.Contains(lower, "api") || strings.Contains(lower, "endpoint")):
		ids := genIDs(5)
		return []Task{
			{ID: ids[0], Title: "Design API schema", Description: "Define the API request/response schema", CreatedAt: now, UpdatedAt: now},
			{ID: ids[1], Title: "Implement endpoint handler", Description: "Implement the API endpoint handler", DependsOn: []string{ids[0]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[2], Title: "Add request validation", Description: "Add input validation logic", DependsOn: []string{ids[1]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[3], Title: "Write API tests", Description: "Write tests for the API endpoint", DependsOn: []string{ids[2]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[4], Title: "Review and finalize", Description: "Review and merge the API changes", DependsOn: []string{ids[3]}, CreatedAt: now, UpdatedAt: now},
		}, nil

	case strings.Contains(lower, "fix") || strings.Contains(lower, "bug"):
		ids := genIDs(4)
		return []Task{
			{ID: ids[0], Title: "Reproduce bug", Description: "Reproduce the reported bug", CreatedAt: now, UpdatedAt: now},
			{ID: ids[1], Title: "Identify root cause", Description: "Debug and find the root cause", DependsOn: []string{ids[0]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[2], Title: "Implement fix", Description: "Implement the bug fix", DependsOn: []string{ids[1]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[3], Title: "Verify fix", Description: "Verify the fix and add regression test", DependsOn: []string{ids[2]}, CreatedAt: now, UpdatedAt: now},
		}, nil

	case strings.Contains(lower, "refactor"):
		ids := genIDs(4)
		return []Task{
			{ID: ids[0], Title: "Analyze current code", Description: "Analyze the code to be refactored", CreatedAt: now, UpdatedAt: now},
			{ID: ids[1], Title: "Plan refactoring", Description: "Create a refactoring plan", DependsOn: []string{ids[0]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[2], Title: "Implement changes", Description: "Execute the refactoring", DependsOn: []string{ids[1]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[3], Title: "Verify behavior", Description: "Verify existing behavior is preserved", DependsOn: []string{ids[2]}, CreatedAt: now, UpdatedAt: now},
		}, nil

	default:
		ids := genIDs(4)
		return []Task{
			{ID: ids[0], Title: "Analyze requirements", Description: fmt.Sprintf("Analyze: %s", goal), CreatedAt: now, UpdatedAt: now},
			{ID: ids[1], Title: "Design solution", Description: "Design the solution approach", DependsOn: []string{ids[0]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[2], Title: "Implement", Description: "Implement the solution", DependsOn: []string{ids[1]}, CreatedAt: now, UpdatedAt: now},
			{ID: ids[3], Title: "Test", Description: "Test the implementation", DependsOn: []string{ids[2]}, CreatedAt: now, UpdatedAt: now},
		}, nil
	}
}

// Schedule assigns unassigned tasks to online agents according to the
// active topology.
//
//	pipeline  – sequential assignment
//	star      – coordinator gets the first task, workers the rest
//	mesh      – round-robin among all online agents
//	hierarchy – distributes by agent role (architect -> developer -> tester -> reviewer)
func (o *Orchestrator) Schedule() error {
	topo := o.store.GetTopology()
	agents := o.store.ListAgents()
	if len(agents) == 0 {
		return fmt.Errorf("no agents registered")
	}

	// Gather unassigned, created tasks in topological order when possible.
	var candidates []*Task
	ordered, err := o.store.TopologicalOrder()
	if err != nil {
		ordered = o.store.ListTasksByStatus(TaskCreated)
	}
	for _, t := range ordered {
		if t.Assignee == "" && t.Status == TaskCreated {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	// Filter to online agents.
	var online []*AgentInfo
	for _, a := range agents {
		if a.Status == AgentOnline {
			online = append(online, a)
		}
	}
	if len(online) == 0 {
		return fmt.Errorf("no online agents available")
	}

	assign := func(t *Task, name string) error {
		return o.store.AssignTask(t.ID, name)
	}

	switch topo.Type {
	case TopoPipeline:
		for i, t := range candidates {
			if err := assign(t, online[i%len(online)].Name); err != nil {
				return err
			}
		}

	case TopoStar:
		if len(topo.Agents) == 0 {
			return fmt.Errorf("star topology requires at least one agent")
		}
		coord := topo.Agents[0]
		workers := topo.Agents[1:]
		for i, t := range candidates {
			target := coord
			if i > 0 && len(workers) > 0 {
				target = workers[(i-1)%len(workers)]
			}
			if err := assign(t, target); err != nil {
				return err
			}
		}

	case TopoMesh:
		for i, t := range candidates {
			if err := assign(t, online[i%len(online)].Name); err != nil {
				return err
			}
		}

	case TopoHierarchy:
		// Bucket online agents by role.
		byRole := map[AgentRole][]string{}
		for _, a := range online {
			byRole[a.Role] = append(byRole[a.Role], a.Name)
		}
		roles := []AgentRole{RoleArchitect, RoleDeveloper, RoleTester, RoleReviewer}
		for i, t := range candidates {
			roleIdx := (i * len(roles)) / len(candidates)
			if roleIdx >= len(roles) {
				roleIdx = len(roles) - 1
			}
			role := roles[roleIdx]
			pool := byRole[role]
			if len(pool) == 0 {
				pool = []string{online[i%len(online)].Name}
			}
			if err := assign(t, pool[i%len(pool)]); err != nil {
				return err
			}
		}

	default:
		// Unknown topology: fall back to round-robin.
		for i, t := range candidates {
			if err := assign(t, online[i%len(online)].Name); err != nil {
				return err
			}
		}
	}

	return nil
}

// Reassign moves a task to a different agent.
func (o *Orchestrator) Reassign(taskID, newAgent string) error {
	return o.store.AssignTask(taskID, newAgent)
}

// CheckTimeout returns tasks that have not reached a terminal status and
// whose UpdatedAt is older than the given duration.
func (o *Orchestrator) CheckTimeout(timeout time.Duration) []*Task {
	now := time.Now()
	statuses := []TaskStatus{
		TaskCreated, TaskAssigned, TaskInProgress, TaskReview, TaskBlocked,
	}
	var timedOut []*Task
	for _, st := range statuses {
		for _, t := range o.store.ListTasksByStatus(st) {
			if now.Sub(t.UpdatedAt) > timeout {
				timedOut = append(timedOut, t)
			}
		}
	}
	return timedOut
}

// Monitor returns a map of task-status -> count for a dashboard overview.
func (o *Orchestrator) Monitor() map[string]int {
	stats := make(map[string]int)
	statuses := []TaskStatus{
		TaskCreated, TaskAssigned, TaskInProgress, TaskReview,
		TaskDone, TaskRejected, TaskBlocked,
	}
	for _, st := range statuses {
		stats[string(st)] = len(o.store.ListTasksByStatus(st))
	}
	return stats
}

// genIDs returns n UUID v4 strings.
func genIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = newUUID()
	}
	return ids
}
