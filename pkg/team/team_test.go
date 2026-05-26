package team

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupStore(t *testing.T) *TeamStore {
	t.Helper()
	store := NewTeamStore()
	_ = store.RegisterAgent(AgentInfo{Name: "alice", Role: RoleDeveloper, Status: AgentOnline})
	_ = store.RegisterAgent(AgentInfo{Name: "bob", Role: RoleTester, Status: AgentOnline})
	_ = store.RegisterAgent(AgentInfo{Name: "carol", Role: RoleArchitect, Status: AgentOnline})
	return store
}

// ---------- 1. RegisterAgent ----------

func TestRegisterAgent(t *testing.T) {
	store := setupStore(t)

	// Re-register updates.
	require.NoError(t, store.RegisterAgent(AgentInfo{Name: "alice", Role: RoleReviewer, Status: AgentBusy}))
	a, ok := store.GetAgent("alice")
	require.True(t, ok)
	assert.Equal(t, RoleReviewer, a.Role)
	assert.NotEmpty(t, a.ID)

	// Empty name rejected.
	assert.Error(t, store.RegisterAgent(AgentInfo{}))

	// ListAgents returns all three.
	assert.Len(t, store.ListAgents(), 3)

	// Non-existent.
	_, ok = store.GetAgent("nobody")
	assert.False(t, ok)
}

// ---------- 2. CreateTask + DetectCycle ----------

func TestCreateTask(t *testing.T) {
	store := setupStore(t)

	assert.Error(t, store.CreateTask(Task{})) // empty title

	require.NoError(t, store.CreateTask(Task{Title: "build widget"}))
	tasks := store.ListTasksByStatus(TaskCreated)
	require.Len(t, tasks, 1)
	assert.NotEmpty(t, tasks[0].ID)

	// Cycle: tA→tB→tC, then tA depends on tC.
	require.NoError(t, store.CreateTask(Task{ID: "tA", Title: "A"}))
	require.NoError(t, store.CreateTask(Task{ID: "tB", Title: "B", DependsOn: []string{"tA"}}))
	require.NoError(t, store.CreateTask(Task{ID: "tC", Title: "C", DependsOn: []string{"tB"}}))
	assert.Error(t, store.DetectCycle("tA", []string{"tC"}))
}

// ---------- 3. TaskStateMachine ----------

func TestTaskStateMachine(t *testing.T) {
	store := setupStore(t)
	require.NoError(t, store.CreateTask(Task{ID: "t1", Title: "fsm"}))

	advance := func(s TaskStatus) { require.NoError(t, store.UpdateTaskStatus("t1", s)) }

	// Valid path: created→assigned→in_progress→review→done.
	advance(TaskAssigned)
	advance(TaskInProgress)
	advance(TaskReview)
	advance(TaskDone)
	assert.Error(t, store.UpdateTaskStatus("t1", TaskCreated)) // done is terminal

	// Rejected path.
	require.NoError(t, store.CreateTask(Task{ID: "t2", Title: "rej"}))
	require.NoError(t, store.UpdateTaskStatus("t2", TaskAssigned))
	require.NoError(t, store.UpdateTaskStatus("t2", TaskRejected))
	require.NoError(t, store.UpdateTaskStatus("t2", TaskCreated)) // rejected→created valid

	// Blocked path.
	require.NoError(t, store.CreateTask(Task{ID: "t3", Title: "blk"}))
	require.NoError(t, store.UpdateTaskStatus("t3", TaskBlocked))
	require.NoError(t, store.UpdateTaskStatus("t3", TaskInProgress)) // blocked→in_progress (no deps)

	// Invalid jumps.
	require.NoError(t, store.CreateTask(Task{ID: "t4", Title: "bad"}))
	assert.Error(t, store.UpdateTaskStatus("t4", TaskDone))
	assert.Error(t, store.UpdateTaskStatus("t4", TaskReview))
	assert.Error(t, store.UpdateTaskStatus("nonexistent", TaskDone))
}

// ---------- 4. TaskDependency ----------

func TestTaskDependency(t *testing.T) {
	store := setupStore(t)
	require.NoError(t, store.CreateTask(Task{ID: "A", Title: "A"}))
	require.NoError(t, store.CreateTask(Task{ID: "B", Title: "B", DependsOn: []string{"A"}}))
	require.NoError(t, store.CreateTask(Task{ID: "C", Title: "C", DependsOn: []string{"B"}}))

	// Block B; can't unblock until A is done.
	require.NoError(t, store.UpdateTaskStatus("B", TaskBlocked))
	assert.Error(t, store.UpdateTaskStatus("B", TaskInProgress))

	// Complete A.
	for _, s := range []TaskStatus{TaskAssigned, TaskInProgress, TaskReview, TaskDone} {
		require.NoError(t, store.UpdateTaskStatus("A", s))
	}
	// Now unblock B.
	assert.NoError(t, store.UpdateTaskStatus("B", TaskInProgress))

	// Topological order: A, B, C.
	ordered, err := store.TopologicalOrder()
	require.NoError(t, err)
	require.Len(t, ordered, 3)
	assert.Equal(t, "A", ordered[0].ID)
	assert.Equal(t, "B", ordered[1].ID)
	assert.Equal(t, "C", ordered[2].ID)
}

// ---------- 5. SendMessage ----------

func TestSendMessage(t *testing.T) {
	store := setupStore(t)

	require.NoError(t, store.SendMessage(Message{From: "alice", To: "bob", Subject: "hello", Body: "hi"}))
	inbox := store.GetInbox("bob")
	require.Len(t, inbox, 1)
	assert.Equal(t, "alice", inbox[0].From)
	assert.False(t, inbox[0].Read)

	assert.Len(t, store.GetInbox("carol"), 0)

	require.NoError(t, store.MarkRead("bob", inbox[0].ID))
	assert.True(t, store.GetInbox("bob")[0].Read)
	assert.Error(t, store.MarkRead("bob", "nonexistent"))
	assert.Error(t, store.SendMessage(Message{From: "alice", Subject: "no to"}))
}

// ---------- 6. Broadcast ----------

func TestBroadcast(t *testing.T) {
	store := setupStore(t)

	require.NoError(t, store.Broadcast("alice", "standup", "daily standup at 10am"))
	for _, name := range []string{"alice", "bob", "carol"} {
		msgs := store.GetInbox(name)
		require.Len(t, msgs, 1, "agent %s", name)
		assert.Equal(t, "alice", msgs[0].From)
		assert.Equal(t, "standup", msgs[0].Subject)
	}
}

// ---------- 7. Topology ----------

func TestTopology(t *testing.T) {
	store := setupStore(t)

	assert.Error(t, store.SetTopology(Topology{Agents: []string{"alice"}}))             // empty type
	assert.Error(t, store.SetTopology(Topology{Type: TopoMesh, Agents: []string{"nobody"}})) // unregistered

	assert.NoError(t, store.SetTopology(Topology{Type: TopoPipeline, Agents: []string{"alice", "bob"}, Options: map[string]string{"stages": "3"}}))
	topo := store.GetTopology()
	assert.Equal(t, TopoPipeline, topo.Type)
	assert.Equal(t, []string{"alice", "bob"}, topo.Agents)
	assert.Equal(t, "3", topo.Options["stages"])
}

// ---------- 8. SharedContext ----------

func TestSharedContext(t *testing.T) {
	store := setupStore(t)

	require.NoError(t, store.PutContext(SharedContext{ID: "ctx1", Authors: []string{"alice"}, Tags: []string{"design", "auth"}, Content: "auth design"}))
	require.NoError(t, store.PutContext(SharedContext{ID: "ctx2", Authors: []string{"bob"}, Tags: []string{"testing"}, Content: "test plan"}))

	assert.Len(t, store.ListContexts("design"), 1)
	assert.Len(t, store.ListContexts("testing"), 1)
	assert.Len(t, store.ListContexts(""), 2)
	assert.Len(t, store.ListContexts("nope"), 0)

	// PurgeExpired.
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	_ = store.PutContext(SharedContext{ID: "old", Authors: []string{"c"}, Tags: []string{"x"}, Content: "old", Expiry: past})
	_ = store.PutContext(SharedContext{ID: "new", Authors: []string{"c"}, Tags: []string{"x"}, Content: "new", Expiry: future})
	_ = store.PutContext(SharedContext{ID: "perm", Authors: []string{"c"}, Tags: []string{"x"}, Content: "perm", Expiry: time.Time{}})

	store.PurgeExpired()
	all := store.ListContexts("")
	assert.Len(t, all, 4) // ctx1, ctx2, new, perm (not old)
	for _, c := range all {
		assert.NotEqual(t, "old", c.ID)
	}
}

// ---------- 9. OrchestratorDecompose ----------

func TestOrchestratorDecompose(t *testing.T) {
	o := NewOrchestrator(NewTeamStore())

	tasks, err := o.DecomposeGoal("Implement login system")
	require.NoError(t, err)
	require.Len(t, tasks, 5)
	assert.Contains(t, tasks[1].DependsOn, tasks[0].ID)

	tasks, err = o.DecomposeGoal("Add API endpoint for users")
	require.NoError(t, err)
	require.Len(t, tasks, 5)
	assert.Equal(t, "Design API schema", tasks[0].Title)

	tasks, err = o.DecomposeGoal("Fix critical bug")
	require.NoError(t, err)
	require.Len(t, tasks, 4)
	assert.Equal(t, "Reproduce bug", tasks[0].Title)

	tasks, err = o.DecomposeGoal("Refactor the database layer")
	require.NoError(t, err)
	require.Len(t, tasks, 4)

	tasks, err = o.DecomposeGoal("make coffee")
	require.NoError(t, err)
	require.Len(t, tasks, 4)
	assert.Equal(t, "Analyze requirements", tasks[0].Title)
}

// ---------- 10. OrchestratorSchedule ----------

func TestOrchestratorSchedule(t *testing.T) {
	// Helper: create store, topology, tasks, schedule, and verify ALL assigned.
	verifyAllAssigned := func(t *testing.T, topo Topology, taskIDs []string) {
		t.Helper()
		store := setupStore(t)
		o := NewOrchestrator(store)
		require.NoError(t, store.SetTopology(topo))
		for _, id := range taskIDs {
			require.NoError(t, store.CreateTask(Task{ID: id, Title: id}))
		}
		require.NoError(t, o.Schedule())
		for _, id := range taskIDs {
			tk, _ := store.GetTask(id)
			assert.NotEmpty(t, tk.Assignee, "task %s should be assigned", id)
		}
	}

	verifyAllAssigned(t, Topology{Type: TopoPipeline, Agents: []string{"alice", "bob"}},
		[]string{"p1", "p2"})
	verifyAllAssigned(t, Topology{Type: TopoStar, Agents: []string{"carol", "alice", "bob"}},
		[]string{"s1", "s2"})
	verifyAllAssigned(t, Topology{Type: TopoMesh, Agents: []string{"alice", "bob", "carol"}},
		[]string{"m1", "m2", "m3"})
	verifyAllAssigned(t, Topology{Type: TopoHierarchy, Agents: []string{"alice", "bob", "carol"}},
		[]string{"h1", "h2"})

	// No online agents.
	store := NewTeamStore()
	_ = store.RegisterAgent(AgentInfo{Name: "off", Role: RoleDeveloper, Status: AgentOffline})
	o := NewOrchestrator(store)
	_ = store.SetTopology(Topology{Type: TopoMesh, Agents: []string{"off"}})
	_ = store.CreateTask(Task{ID: "t", Title: "stuck"})
	assert.Error(t, o.Schedule())

	// No candidates.
	store2 := setupStore(t)
	o2 := NewOrchestrator(store2)
	_ = store2.SetTopology(Topology{Type: TopoMesh, Agents: []string{"alice"}})
	assert.NoError(t, o2.Schedule())

	// CheckTimeout.
	store3 := setupStore(t)
	o3 := NewOrchestrator(store3)
	_ = store3.CreateTask(Task{ID: "stale", Title: "stale"})
	assert.Len(t, o3.CheckTimeout(time.Nanosecond), 1)
	assert.Len(t, o3.CheckTimeout(24*time.Hour), 0)

	// Empty store.
	assert.Error(t, NewOrchestrator(NewTeamStore()).Schedule())
}
