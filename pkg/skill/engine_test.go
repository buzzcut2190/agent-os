package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestParseState(t *testing.T) {
	tests := []struct {
		input    string
		expected SkillState
	}{
		{"on", SkillActive},
		{"active", SkillActive},
		{"1", SkillActive},
		{"true", SkillActive},
		{"off", SkillInactive},
		{"inactive", SkillInactive},
		{"0", SkillInactive},
		{"false", SkillInactive},
		{"invalid", SkillError},
		{"", SkillError},
	}
	for _, tt := range tests {
		got := ParseState(tt.input)
		if got != tt.expected {
			t.Errorf("ParseState(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestSkillState_String(t *testing.T) {
	tests := []struct {
		state    SkillState
		expected string
	}{
		{SkillActive, "on"},
		{SkillInactive, "off"},
		{SkillError, "error"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("SkillState(%d).String() = %q, want %q", tt.state, got, tt.expected)
		}
	}
}

func TestNewEngine(t *testing.T) {
	e := NewEngine("/tmp/skill-test", "test-project")
	if e == nil {
		t.Fatal("NewEngine returned nil")
	}
	if e.projectName != "test-project" {
		t.Errorf("projectName = %q, want %q", e.projectName, "test-project")
	}
	if e.stateDir != "/tmp/skill-test" {
		t.Errorf("stateDir = %q, want %q", e.stateDir, "/tmp/skill-test")
	}
}

func TestEngine_LoadBuiltinSkills(t *testing.T) {
	e := NewEngine("", "test")
	if err := e.LoadFromFS(BuiltinSkillsFS(), "builtin"); err != nil {
		t.Fatalf("LoadFromFS: %v", err)
	}
	skills := e.List()
	if len(skills) == 0 {
		t.Fatal("expected at least 1 builtin skill")
	}
	// Verify feedback-synthesis loaded.
	found := false
	for _, s := range skills {
		if s.Name == "feedback-synthesis" {
			found = true
			if s.Source != "builtin" {
				t.Errorf("source = %q, want builtin", s.Source)
			}
			if s.State != SkillInactive {
				t.Errorf("initial state = %v, want inactive", s.State)
			}
		}
	}
	if !found {
		t.Error("feedback-synthesis skill not found in loaded skills")
	}
}

func TestEngine_ActivateDeactivate(t *testing.T) {
	e := NewEngine("", "test")
	e.LoadFromFS(BuiltinSkillsFS(), "builtin")

	// Activate.
	if err := e.Activate("feedback-synthesis"); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !e.IsActive("feedback-synthesis") {
		t.Error("expected skill to be active")
	}

	// Idempotent re-activate.
	if err := e.Activate("feedback-synthesis"); err != nil {
		t.Errorf("re-Activate should be idempotent: %v", err)
	}

	// Deactivate.
	if err := e.Deactivate("feedback-synthesis"); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if e.IsActive("feedback-synthesis") {
		t.Error("expected skill to be inactive")
	}

	// Idempotent re-deactivate.
	if err := e.Deactivate("feedback-synthesis"); err != nil {
		t.Errorf("re-Deactivate should be idempotent: %v", err)
	}
}

func TestEngine_ActivateNotFound(t *testing.T) {
	e := NewEngine("", "test")
	err := e.Activate("nonexistent")
	if err == nil {
		t.Error("expected error activating nonexistent skill")
	}
}

func TestEngine_DeactivateNotFound(t *testing.T) {
	e := NewEngine("", "test")
	err := e.Deactivate("nonexistent")
	if err == nil {
		t.Error("expected error deactivating nonexistent skill")
	}
}

func TestEngine_Active(t *testing.T) {
	e := NewEngine("", "test")
	e.LoadFromFS(BuiltinSkillsFS(), "builtin")
	e.Activate("feedback-synthesis")

	active := e.Active()
	if len(active) != 1 {
		t.Fatalf("expected 1 active skill, got %d", len(active))
	}
	if active[0].Name != "feedback-synthesis" {
		t.Errorf("active skill name = %q, want feedback-synthesis", active[0].Name)
	}
	if active[0].State != SkillActive {
		t.Errorf("active skill state = %v, want active", active[0].State)
	}
}

func TestEngine_GetContext(t *testing.T) {
	e := NewEngine("", "test-project")
	e.SetLangProvider(func() string { return "go" })
	e.SetStructProvider(func() string { return "src/\n  main.go\n  pkg/" })
	e.LoadFromFS(BuiltinSkillsFS(), "builtin")

	ctx, err := e.GetContext("feedback-synthesis")
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}
	if ctx == "" {
		t.Error("expected non-empty context")
	}
	if !contains(ctx, "test-project") {
		t.Error("context should contain project name")
	}
	if !contains(ctx, "go") {
		t.Error("context should contain project language")
	}
}

func TestEngine_GetContextNotFound(t *testing.T) {
	e := NewEngine("", "test")
	_, err := e.GetContext("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestEngine_GetMetadata(t *testing.T) {
	e := NewEngine("", "test")
	e.LoadFromFS(BuiltinSkillsFS(), "builtin")

	data, err := e.GetMetadata("feedback-synthesis")
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}
	var info SkillInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if info.Name != "feedback-synthesis" {
		t.Errorf("name = %q, want feedback-synthesis", info.Name)
	}
	if info.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", info.Version)
	}
}

func TestEngine_GetPrompt(t *testing.T) {
	e := NewEngine("", "test")
	e.LoadFromFS(BuiltinSkillsFS(), "builtin")

	// Rendered.
	rendered, err := e.GetPrompt("feedback-synthesis", true)
	if err != nil {
		t.Fatalf("GetPrompt(rendered): %v", err)
	}
	if rendered == "" {
		t.Error("expected non-empty rendered prompt")
	}

	// Raw template.
	raw, err := e.GetPrompt("feedback-synthesis", false)
	if err != nil {
		t.Fatalf("GetPrompt(raw): %v", err)
	}
	if raw == "" {
		t.Error("expected non-empty raw prompt")
	}
}

func TestEngine_LoadAll(t *testing.T) {
	// Create a temporary project skill dir.
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(`name: my-skill
description: A test skill
version: 0.1.0
author: test
tags: [test]
lang: en
`), 0644)
	os.WriteFile(filepath.Join(skillDir, "context.md"), []byte("Context for {{.ProjectName}}"), 0644)
	os.WriteFile(filepath.Join(skillDir, "prompt.md"), []byte("Prompt for {{.ProjectName}}"), 0644)

	e := NewEngine("", "test", tmpDir)
	if err := e.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	skills := e.List()
	// Should have both builtin and the project skill.
	found := false
	for _, s := range skills {
		if s.Name == "my-skill" {
			found = true
			if s.Source != "project" {
				t.Errorf("my-skill source = %q, want project", s.Source)
			}
		}
	}
	if !found {
		t.Error("my-skill not found after LoadAll")
	}
}

func TestEngine_Discover(t *testing.T) {
	e := NewEngine("", "test")
	e.LoadFromFS(BuiltinSkillsFS(), "builtin")

	tmpDir := t.TempDir()
	newSkill := filepath.Join(tmpDir, "new-skill")
	os.MkdirAll(newSkill, 0755)
	os.WriteFile(filepath.Join(newSkill, "skill.yaml"), []byte("name: new-skill\ndescription: New\nversion: 1.0\n"), 0644)

	discovered, err := e.Discover(tmpDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(discovered) != 1 || discovered[0] != "new-skill" {
		t.Errorf("Discover() = %v, want [new-skill]", discovered)
	}
}

func TestEngine_SaveLoadState(t *testing.T) {
	stateDir := t.TempDir()
	e := NewEngine(stateDir, "test")
	e.LoadFromFS(BuiltinSkillsFS(), "builtin")
	e.Activate("feedback-synthesis")

	if err := e.SaveState(); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Verify file exists.
	statePath := filepath.Join(stateDir, "skills-state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal("state file was not created")
	}

	// Create new engine and load state.
	e2 := NewEngine(stateDir, "test")
	e2.LoadFromFS(BuiltinSkillsFS(), "builtin")
	if err := e2.LoadState(); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !e2.IsActive("feedback-synthesis") {
		t.Error("expected feedback-synthesis to be active after state reload")
	}
}

func TestEngine_ConcurrentAccess(t *testing.T) {
	e := NewEngine("", "test")
	e.LoadFromFS(BuiltinSkillsFS(), "builtin")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Activate("feedback-synthesis")
			e.List()
			e.Active()
			e.IsActive("feedback-synthesis")
			e.GetContext("feedback-synthesis")
			e.Deactivate("feedback-synthesis")
		}()
	}
	wg.Wait()
	// No panics or deadlocks = success.
}

func TestEngine_GracefulDegradation(t *testing.T) {
	// Create a skill with broken yaml.
	tmpDir := t.TempDir()
	brokenDir := filepath.Join(tmpDir, "broken-skill")
	os.MkdirAll(brokenDir, 0755)
	os.WriteFile(filepath.Join(brokenDir, "skill.yaml"), []byte("name: broken\ninvalid_yaml: [unclosed"), 0644)

	e := NewEngine("", "test")
	e.LoadFromDir(tmpDir, "project")

	// Should still have loaded it with an error state.
	s, err := e.Get("broken-skill")
	if err != nil {
		t.Fatalf("Get should return the broken skill, not error: %v", err)
	}
	if s.State != SkillError {
		t.Errorf("broken skill state = %v, want SkillError", s.State)
	}
	if s.LoadError == nil {
		t.Error("expected LoadError to be set")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && hasSubstr(s, substr))
}

func hasSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
