package kernel

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLifecycleManager_Spawn(t *testing.T) {
	lm := NewLifecycleManager()
	id, err := lm.Spawn(AgentTypeCodeReview, AgentConfig{Model: "deepseek-chat"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty agent ID")
	}
	a, ok := lm.Get(id)
	if !ok {
		t.Fatal("Get returned false")
	}
	if a.Type != AgentTypeCodeReview {
		t.Errorf("type = %s, want code-reviewer", a.Type)
	}
	if a.State != AgentCreated {
		t.Errorf("initial state = %s, want created", a.State)
	}
}

func TestLifecycleManager_Run(t *testing.T) {
	lm := NewLifecycleManager()
	id, _ := lm.Spawn(AgentTypeTester, AgentConfig{})
	if err := lm.Run(id); err != nil {
		t.Fatalf("Run: %v", err)
	}
	a, _ := lm.Get(id)
	if a.State != AgentRunning {
		t.Errorf("state = %s, want running", a.State)
	}
}

func TestLifecycleManager_SuspendResume(t *testing.T) {
	lm := NewLifecycleManager()
	id, _ := lm.Spawn(AgentTypeDeveloper, AgentConfig{})
	lm.Run(id)
	lm.Suspend(id)
	a, _ := lm.Get(id)
	if a.State != AgentSuspended {
		t.Errorf("state = %s, want suspended", a.State)
	}
	lm.Resume(id)
	a, _ = lm.Get(id)
	if a.State != AgentRunning {
		t.Errorf("state = %s, want running", a.State)
	}
}

func TestLifecycleManager_Kill(t *testing.T) {
	lm := NewLifecycleManager()
	id, _ := lm.Spawn(AgentTypeMonitor, AgentConfig{})
	lm.Kill(id)
	a, _ := lm.Get(id)
	if a.State != AgentTerminated {
		t.Errorf("state = %s, want terminated", a.State)
	}
}

func TestLifecycleManager_KillAll(t *testing.T) {
	lm := NewLifecycleManager()
	for i := 0; i < 10; i++ {
		lm.Spawn(AgentTypeTester, AgentConfig{})
	}
	if n := lm.KillAll(); n != 10 {
		t.Errorf("KillAll = %d, want 10", n)
	}
}

func TestLifecycleManager_List(t *testing.T) {
	lm := NewLifecycleManager()
	lm.Spawn(AgentTypeCodeReview, AgentConfig{})
	lm.Spawn(AgentTypeTester, AgentConfig{})
	lm.Spawn(AgentTypeArchitect, AgentConfig{})
	if n := len(lm.List("")); n != 3 {
		t.Errorf("List() = %d, want 3", n)
	}
	running := lm.List(AgentRunning)
	if len(running) != 0 {
		t.Errorf("expected 0 running, got %d", len(running))
	}
}

func TestLifecycleManager_ConcurrentSpawn(t *testing.T) {
	lm := NewLifecycleManager()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.Spawn(AgentTypeTester, AgentConfig{})
		}()
	}
	wg.Wait()
	if n := len(lm.List("")); n != 50 {
		t.Errorf("expected 50 agents, got %d", n)
	}
}

func TestStateStore_SaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(filepath.Join(tmpDir, "agents.jsonl"))
	lm := NewLifecycleManager()
	id, _ := lm.Spawn(AgentTypeCodeReview, AgentConfig{Model: "test-model"})

	if err := store.Snapshot(lm); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	lm2 := NewLifecycleManager()
	if err := store.Restore(lm2); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	a, ok := lm2.Get(id)
	if !ok {
		t.Fatal("restored agent not found")
	}
	if a.Config.Model != "test-model" {
		t.Errorf("model = %q, want test-model", a.Config.Model)
	}
}

func TestScheduler_Submit(t *testing.T) {
	lm := NewLifecycleManager()
	s := NewScheduler(lm)
	task := Task{Name: "review PR #42", Priority: 80, AgentType: AgentTypeCodeReview}
	id, err := s.Submit(task)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty task ID")
	}
	pending := s.ListPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending task, got %d", len(pending))
	}
}

func TestScheduler_PriorityOrder(t *testing.T) {
	lm := NewLifecycleManager()
	s := NewScheduler(lm)
	s.Submit(Task{Name: "low", Priority: 10, AgentType: AgentTypeTester})
	s.Submit(Task{Name: "high", Priority: 90, AgentType: AgentTypeTester})
	s.Submit(Task{Name: "mid", Priority: 50, AgentType: AgentTypeTester})
	pending := s.ListPending()
	if pending[0].Priority != 90 {
		t.Errorf("first pending priority = %d, want 90", pending[0].Priority)
	}
}

func TestScheduler_Cancel(t *testing.T) {
	lm := NewLifecycleManager()
	s := NewScheduler(lm)
	id, _ := s.Submit(Task{Name: "test", Priority: 50, AgentType: AgentTypeTester})
	if err := s.Cancel(id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if len(s.ListPending()) != 0 {
		t.Error("expected 0 pending after cancel")
	}
}

func TestIPC_SendReceive(t *testing.T) {
	ipc := NewIPC()
	ch := ipc.Subscribe("agent-1", "build")
	defer ipc.Unsubscribe("agent-1", "build")

	sent := ipc.Send("agent-2", "agent-1", "build", "test", "hello")
	if sent != nil {
		t.Fatalf("Send: %v", sent)
	}
	select {
	case msg := <-ch:
		if msg.Body != "hello" {
			t.Errorf("body = %q, want hello", msg.Body)
		}
	default:
		t.Error("expected message on channel")
	}
}

func TestIPC_Broadcast(t *testing.T) {
	ipc := NewIPC()
	ch1 := ipc.Subscribe("a", "general")
	ch2 := ipc.Subscribe("b", "general")
	defer ipc.Unsubscribe("a", "general")
	defer ipc.Unsubscribe("b", "general")

	delivered := ipc.Broadcast("sender", "general", "test", "broadcast msg")
	if len(delivered) != 2 {
		t.Errorf("delivered to %d, want 2", len(delivered))
	}
	<-ch1
	<-ch2
}

func TestResourceManager_AllocateRelease(t *testing.T) {
	rm := NewResourceManager(ResourceLimits{MaxAgents: 2, MaxTotalTokens: 1000, MaxTokensPerAgent: 500})
	if err := rm.Allocate("a", ResourceUsage{TokensUsed: 300}); err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if err := rm.Allocate("b", ResourceUsage{TokensUsed: 300}); err != nil {
		t.Fatalf("Allocate b: %v", err)
	}
	// Third should fail.
	if err := rm.Allocate("c", ResourceUsage{TokensUsed: 300}); err == nil {
		t.Error("expected resource exhausted error")
	}
	rm.Release("a")
	if err := rm.Allocate("c", ResourceUsage{TokensUsed: 300}); err != nil {
		t.Errorf("Allocate after release: %v", err)
	}
}

func TestModelRouter_Route(t *testing.T) {
	mr := NewModelRouter(DefaultRoutingRules())
	model, provider := mr.Route(Task{AgentType: AgentTypeArchitect, Priority: 80})
	if model != "deepseek-reasoner" {
		t.Errorf("architect model = %q, want deepseek-reasoner", model)
	}
	if provider != "deepseek" {
		t.Errorf("provider = %q, want deepseek", provider)
	}
	model, _ = mr.Route(Task{AgentType: AgentTypeCodeReview, Priority: 10})
	if model != "deepseek-chat" {
		t.Errorf("code-review model = %q, want deepseek-chat", model)
	}
}

func TestContextCache_HitMiss(t *testing.T) {
	c := NewContextCache(10, 0)
	c.Set("key1", []byte("value1"))
	val, ok := c.Get("key1")
	if !ok || string(val) != "value1" {
		t.Error("cache miss on existing key")
	}
	_, ok = c.Get("nonexistent")
	if ok {
		t.Error("cache hit on missing key")
	}
}

func TestContextCache_LRU(t *testing.T) {
	c := NewContextCache(2, 0)
	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3")) // Should evict "a"
	if _, ok := c.Get("a"); ok {
		t.Error("a should have been evicted")
	}
	if _, ok := c.Get("b"); !ok {
		t.Error("b should still be cached")
	}
}

func TestWaitGroup(t *testing.T) {
	wg := NewWaitGroup()
	wg.Add("a1")
	wg.Add("a2")
	go func() { wg.Done("a1", true, "") }()
	go func() { wg.Done("a2", false, "timeout") }()
	results := wg.Wait()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Success && !results[1].Success {
		if results[0].Error != "timeout" && results[1].Error != "timeout" {
			t.Error("expected one result with timeout error")
		}
	}
}

func TestStateStore_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state", "agents.jsonl")
	store := NewStateStore(statePath)
	lm := NewLifecycleManager()
	lm.Spawn(AgentTypeMonitor, AgentConfig{})
	store.Snapshot(lm)
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file not created: %v", err)
	}
}
