package workspace

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewEngine(t *testing.T) {
	tmpDir := t.TempDir()
	eng, err := NewEngine(tmpDir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}
	if len(eng.List()) != 0 {
		t.Errorf("expected 0 workspaces, got %d", len(eng.List()))
	}
}

func TestEngine_Create(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	ws, err := eng.Create("agent-1", TypeAgent, WorkspaceConfig{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ws.ID == "" {
		t.Error("expected non-empty workspace ID")
	}
	if ws.OwnerID != "agent-1" {
		t.Errorf("OwnerID = %s, want agent-1", ws.OwnerID)
	}
	if ws.Status != StatusActive {
		t.Errorf("Status = %s, want active", ws.Status)
	}
	root := filepath.Join(tmpDir, string(ws.ID))
	if _, err := os.Stat(filepath.Join(root, "home", "cache")); os.IsNotExist(err) {
		t.Error("missing home/cache")
	}
}

func TestEngine_CreateTeam(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	ws, err := eng.Create("orchestrator-1", TypeTeam, WorkspaceConfig{
		MaxArtifacts: 100, Labels: map[string]string{"project": "agent-os"},
	})
	if err != nil {
		t.Fatalf("Create team: %v", err)
	}
	if ws.Type != TypeTeam {
		t.Errorf("Type = %s, want team", ws.Type)
	}
}

func TestEngine_Get(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	created, _ := eng.Create("agent-1", TypeAgent, WorkspaceConfig{})
	got, err := eng.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %s, want %s", got.ID, created.ID)
	}
}

func TestEngine_Get_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	_, err := eng.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent workspace")
	}
}

func TestEngine_List(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	eng.Create("a", TypeAgent, WorkspaceConfig{})
	eng.Create("b", TypeAgent, WorkspaceConfig{})
	eng.Create("c", TypeTeam, WorkspaceConfig{})
	if len(eng.List()) != 3 {
		t.Errorf("expected 3 workspaces, got %d", len(eng.List()))
	}
}

func TestEngine_Archive(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	ws, _ := eng.Create("agent-1", TypeAgent, WorkspaceConfig{})
	eng.Archive(ws.ID)
	got, _ := eng.Get(ws.ID)
	if got.Status != StatusArchived {
		t.Errorf("Status = %s, want archived", got.Status)
	}
}

func TestEngine_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	ws, _ := eng.Create("agent-1", TypeAgent, WorkspaceConfig{})
	eng.Delete(ws.ID)
	for _, w := range eng.List() {
		if w.ID == ws.ID {
			t.Error("deleted workspace should not appear in List()")
		}
	}
	_, err := eng.Get(ws.ID)
	if err == nil {
		t.Error("expected error getting deleted workspace")
	}
}

func TestEngine_GetByOwner(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	eng.Create("agent-a", TypeAgent, WorkspaceConfig{})
	eng.Create("agent-a", TypeAgent, WorkspaceConfig{})
	eng.Create("agent-b", TypeAgent, WorkspaceConfig{})
	if len(eng.GetByOwner("agent-a")) != 2 {
		t.Error("agent-a should have 2 workspaces")
	}
	if len(eng.GetByOwner("agent-b")) != 1 {
		t.Error("agent-b should have 1 workspace")
	}
}

func TestEngine_PathHelpers(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	ws, _ := eng.Create("agent-1", TypeAgent, WorkspaceConfig{})
	home, _ := eng.HomePath(ws.ID)
	if !filepath.IsAbs(home) {
		t.Error("HomePath should be absolute")
	}
	scratch, _ := eng.ScratchPath(ws.ID)
	if !filepath.IsAbs(scratch) {
		t.Error("ScratchPath should be absolute")
	}
}

func TestEngine_RegistryPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	ws, _ := eng.Create("agent-x", TypeAgent, WorkspaceConfig{})
	eng2, _ := NewEngine(tmpDir)
	got, err := eng2.Get(ws.ID)
	if err != nil {
		t.Fatalf("reloaded workspace not found: %v", err)
	}
	if got.OwnerID != "agent-x" {
		t.Errorf("OwnerID = %s, want agent-x", got.OwnerID)
	}
}

func TestEngine_ConcurrentCreate(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	var wg sync.WaitGroup
	ids := make(chan WorkspaceID, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ws, err := eng.Create("agent-c", TypeAgent, WorkspaceConfig{})
			if err == nil {
				ids <- ws.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := make(map[WorkspaceID]bool)
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate workspace ID: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != 100 {
		t.Errorf("expected 100 unique IDs, got %d", len(seen))
	}
}

func TestHomeDir_ConfigReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	h := NewHomeDir(tmpDir)

	cfg := map[string]any{"model": "deepseek-chat", "max_tokens": 100000}
	if err := h.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	got, err := h.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got["model"] != "deepseek-chat" {
		t.Errorf("model = %v, want deepseek-chat", got["model"])
	}
}

func TestHomeDir_ProfileReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	h := NewHomeDir(tmpDir)

	profile := &AgentProfile{
		Name: "CodeReviewer-3000", Language: "zh",
		PreferredModel: "deepseek-chat", Style: StyleMinimal,
	}
	if err := h.SetProfile(profile); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}
	got, err := h.GetProfile()
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Name != "CodeReviewer-3000" {
		t.Errorf("Name = %s, want CodeReviewer-3000", got.Name)
	}
	if got.Style != StyleMinimal {
		t.Errorf("Style = %s, want minimal", got.Style)
	}
}

func TestHomeDir_CacheGetSet(t *testing.T) {
	tmpDir := t.TempDir()
	h := NewHomeDir(tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, "cache"), 0755)

	err := h.CacheSet("key1", []byte("cached-value"), 1*time.Hour)
	if err != nil {
		t.Fatalf("CacheSet: %v", err)
	}
	val, err := h.CacheGet("key1")
	if err != nil {
		t.Fatalf("CacheGet: %v", err)
	}
	if string(val) != "cached-value" {
		t.Errorf("cache value = %s, want cached-value", string(val))
	}
	_, err = h.CacheGet("nonexistent")
	if err == nil {
		t.Error("expected error on cache miss")
	}
}

func TestHomeDir_CacheExpiry(t *testing.T) {
	tmpDir := t.TempDir()
	h := NewHomeDir(tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, "cache"), 0755)
	// Set with 0 TTL for immediate expiry.
	h.CacheSet("key-ttl", []byte("value"), 0)
	// With TTL=0, entry should not expire (zero means never-expire).
	// Test by setting 1ns TTL which rounds to same-second expiry.
	h.CacheSet("key-expire", []byte("expire-value"), 1*time.Nanosecond)
	time.Sleep(1100 * time.Millisecond)
	_, err := h.CacheGet("key-expire")
	if err == nil {
		t.Error("cache should have expired after 1s+ sleep")
	}
}

func TestHomeDir_CacheClear(t *testing.T) {
	tmpDir := t.TempDir()
	h := NewHomeDir(tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, "cache"), 0755)
	h.CacheSet("a", []byte("1"), 1*time.Hour)
	h.CacheSet("b", []byte("2"), 1*time.Hour)
	h.CacheClear()
	_, err := h.CacheGet("a")
	if err == nil {
		t.Error("cache should be cleared")
	}
}

func TestScratchArea_WriteRead(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewScratchArea(tmpDir)

	if err := s.Write("test.txt", []byte("hello scratch")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := s.Read("test.txt")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(data) != "hello scratch" {
		t.Errorf("data = %s, want hello scratch", string(data))
	}
}

func TestScratchArea_List(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewScratchArea(tmpDir)
	s.Write("a.txt", []byte("a"))
	s.Write("b.txt", []byte("b"))
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 files, got %d", len(list))
	}
}

func TestScratchArea_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewScratchArea(tmpDir)
	s.Write("old.txt", []byte("old"))
	time.Sleep(10 * time.Millisecond)
	if err := s.Cleanup(1 * time.Millisecond); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	data, _ := s.Read("old.txt")
	if data != nil {
		t.Error("old file should have been cleaned up")
	}
}

func TestArtifactStore_SaveGet(t *testing.T) {
	tmpDir := t.TempDir()
	a := NewArtifactStore(tmpDir)

	art := Artifact{Name: "review-pr42", Type: ArtifactReport, Content: "## Code Review: PR #42"}
	if err := a.Save("review-pr42", art); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := a.Get("review-pr42")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "review-pr42" {
		t.Errorf("Name = %s, want review-pr42", got.Name)
	}
	if got.Type != ArtifactReport {
		t.Errorf("Type = %s, want report", got.Type)
	}
}

func TestArtifactStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	a := NewArtifactStore(tmpDir)
	a.Save("art-1", Artifact{Name: "art-1", Type: ArtifactDiff, Content: "diff"})
	a.Save("art-2", Artifact{Name: "art-2", Type: ArtifactCode, Content: "code"})
	index, err := a.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(index) != 2 {
		t.Errorf("expected 2 artifacts, got %d", len(index))
	}
}

func TestAgentMemory_LearnRecall(t *testing.T) {
	tmpDir := t.TempDir()
	am := NewAgentMemory(tmpDir)

	am.Learn("Go 1.25", "Go 1.25 supports range-over-func")
	am.Learn("FUSE tip", "Always check inode ranges before passthrough")

	recalled, err := am.Recall("FUSE")
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if !containsStr(recalled, "FUSE tip") {
		t.Error("recall should find FUSE-related learning")
	}
	if containsStr(recalled, "range-over-func") {
		t.Error("recall for FUSE should not return Go learning")
	}
}

func TestAgentMemory_RecallEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	am := NewAgentMemory(tmpDir)
	content, err := am.Recall("anything")
	if err != nil {
		t.Fatalf("Recall on empty: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty string, got %q", content)
	}
}

func TestAgentMemory_LogSession(t *testing.T) {
	tmpDir := t.TempDir()
	am := NewAgentMemory(tmpDir)
	am.LogSession("Completed Phase 10 kernel development")
	time.Sleep(1100 * time.Millisecond) // Session files are named by second.
	am.LogSession("Started Phase 11 daemon implementation")
	sessions, err := am.RecentSessions(10)
	if err != nil {
		t.Fatalf("RecentSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestTeamWorkspace_Members(t *testing.T) {
	tmpDir := t.TempDir()
	ws := &Workspace{ID: "team-1", OwnerID: "orchestrator", Type: TypeTeam, Status: StatusActive}
	tw := NewTeamWorkspace(ws, tmpDir)
	tw.AddMember("architect-1")
	tw.AddMember("dev-1")
	tw.AddMember("dev-2")
	if len(tw.Members()) != 3 {
		t.Errorf("expected 3 members, got %d", len(tw.Members()))
	}
	tw.RemoveMember("dev-2")
	if len(tw.Members()) != 2 {
		t.Error("expected 2 members after remove")
	}
}

func TestTeamWorkspace_Broadcast(t *testing.T) {
	tmpDir := t.TempDir()
	ws := &Workspace{ID: "team-1", Type: TypeTeam, Status: StatusActive}
	tw := NewTeamWorkspace(ws, tmpDir)

	ch := tw.BroadcastChannel()
	go func() {
		select {
		case msg, ok := <-ch:
			if ok && msg.Body != "hello team" {
				t.Errorf("broadcast body = %s, want hello team", msg.Body)
			}
		case <-time.After(1 * time.Second):
		}
	}()
	time.Sleep(10 * time.Millisecond)
	tw.Broadcast("architect", "general", "hello team")
}

func TestTeamWorkspace_SharedContext(t *testing.T) {
	tmpDir := t.TempDir()
	ws := &Workspace{ID: "team-1", Type: TypeTeam, Status: StatusActive}
	tw := NewTeamWorkspace(ws, tmpDir)

	ctx := &SharedContext{
		CurrentGoal: "Complete Phase 12", Architecture: "Microservices with FUSE",
		Conventions: []string{"Go 1.25", "tab indentation"}, ActiveBranches: []string{"main"},
	}
	if err := tw.SetSharedContext(ctx); err != nil {
		t.Fatalf("SetSharedContext: %v", err)
	}
	got := tw.GetSharedContext()
	if got.CurrentGoal != "Complete Phase 12" {
		t.Errorf("CurrentGoal = %s, want Complete Phase 12", got.CurrentGoal)
	}
}

func TestTeamWorkspace_Decisions(t *testing.T) {
	tmpDir := t.TempDir()
	ws := &Workspace{ID: "team-1", Type: TypeTeam, Status: StatusActive}
	tw := NewTeamWorkspace(ws, tmpDir)

	d, err := tw.AddDecision(Decision{
		Title: "Use JSONL", Decision: "JSONL storage", Reasoning: "Simple, appendable.",
	})
	if err != nil {
		t.Fatalf("AddDecision: %v", err)
	}
	if d.Status != DecisionActive {
		t.Errorf("Status = %s, want active", d.Status)
	}

	decisions := tw.GetDecisions()
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	tw.SupersedeDecision(d.ID)
	decisions = tw.GetDecisions()
	if decisions[0].Status != DecisionSuperseded {
		t.Errorf("Status = %s, want superseded", decisions[0].Status)
	}

	d2, _ := tw.AddDecision(Decision{Title: "Use tabs", Decision: "tabs", Reasoning: "Go standard"})
	tw.RevertDecision(d2.ID)
	decisions = tw.GetDecisions()
	for _, dec := range decisions {
		if dec.Title == "Use tabs" && dec.Status != DecisionReverted {
			t.Error("reverted decision should have reverted status")
		}
	}
}

func TestTeamWorkspace_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	ws := &Workspace{ID: "team-1", Type: TypeTeam, Status: StatusActive}
	tw := NewTeamWorkspace(ws, tmpDir)
	tw.AddMember("a1")
	tw.AddDecision(Decision{Title: "test", Decision: "ok", Reasoning: "yes"})

	tw2 := NewTeamWorkspace(ws, tmpDir)
	if len(tw2.Members()) != 1 || tw2.Members()[0] != "a1" {
		t.Error("members not persisted")
	}
	decisions := tw2.GetDecisions()
	if len(decisions) != 1 || decisions[0].Title != "test" {
		t.Error("decisions not persisted")
	}
}

func TestEngine_CreateTemp(t *testing.T) {
	tmpDir := t.TempDir()
	eng, _ := NewEngine(tmpDir)
	ws, err := eng.Create("agent-t", TypeTemp, WorkspaceConfig{TTL: "24h"})
	if err != nil {
		t.Fatalf("Create temp: %v", err)
	}
	if ws.Type != TypeTemp {
		t.Errorf("Type = %s, want temp", ws.Type)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
