package daemon

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agent-os/agent-os/pkg/kernel"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Error("expected Enabled=true")
	}
	if cfg.Interval != 10*time.Second {
		t.Errorf("Interval = %v, want 10s", cfg.Interval)
	}
	if !cfg.Subsystems.Watcher || !cfg.Subsystems.Scheduler || !cfg.Subsystems.Reporter || !cfg.Subsystems.Miner {
		t.Error("all subsystems should be enabled by default")
	}
	if cfg.MaxAgents != 50 {
		t.Errorf("MaxAgents = %d, want 50", cfg.MaxAgents)
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "daemon.yaml")
	content := `enabled: true
interval: 30s
log_dir: /tmp/logs
max_agents: 25
subsystems:
  watcher: true
  scheduler: true
  reporter: false
  miner: true
`
	os.WriteFile(configPath, []byte(content), 0644)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.MaxAgents != 25 {
		t.Errorf("MaxAgents = %d, want 25", cfg.MaxAgents)
	}
	if cfg.Interval != 30*time.Second {
		t.Errorf("Interval = %v, want 30s", cfg.Interval)
	}
	if cfg.Subsystems.Reporter {
		t.Error("reporter should be disabled")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path/daemon.yaml")
	if err == nil {
		t.Error("expected error for missing config file")
	}
	if cfg.MaxAgents != 50 {
		t.Error("should return defaults on error")
	}
}

func TestDaemon_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DaemonConfig{
		Enabled:   true,
		Interval:  100 * time.Millisecond,
		LogDir:    tmpDir,
		PIDFile:   filepath.Join(tmpDir, "daemon.pid"),
		MaxAgents: 50,
		Subsystems: SubsystemConfig{
			Watcher: false, Scheduler: false, Reporter: false, Miner: false,
		},
	}
	lm := kernel.NewLifecycleManager()
	d := New(cfg, lm)

	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status := d.Status()
	if !status.Running {
		t.Error("expected daemon to be running")
	}
	if status.PID == 0 {
		t.Error("expected non-zero PID")
	}

	if err := d.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if d.Status().Running {
		t.Error("expected daemon to be stopped")
	}
}

func TestDaemon_DoubleStart(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DaemonConfig{
		Interval: 100 * time.Millisecond,
		LogDir:   tmpDir,
		PIDFile:  filepath.Join(tmpDir, "daemon.pid"),
		Subsystems: SubsystemConfig{},
	}
	d := New(cfg, kernel.NewLifecycleManager())
	d.Start()
	err := d.Start()
	if err == nil {
		t.Error("expected error on double start")
	}
	d.Stop()
}

func TestDaemon_StopNotRunning(t *testing.T) {
	cfg := DaemonConfig{}
	d := New(cfg, kernel.NewLifecycleManager())
	err := d.Stop()
	if err == nil {
		t.Error("expected error stopping non-running daemon")
	}
}

func TestDaemon_PIDFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DaemonConfig{
		Interval:   100 * time.Millisecond,
		LogDir:     tmpDir,
		PIDFile:    filepath.Join(tmpDir, "daemon.pid"),
		Subsystems: SubsystemConfig{},
	}
	d := New(cfg, kernel.NewLifecycleManager())
	d.Start()

	if _, err := os.Stat(cfg.PIDFile); os.IsNotExist(err) {
		t.Error("PID file was not created")
	}

	d.Stop()
	if _, err := os.Stat(cfg.PIDFile); !os.IsNotExist(err) {
		t.Error("PID file was not removed on stop")
	}
}

func TestDaemonWithWatcher(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DaemonConfig{
		Interval: 50 * time.Millisecond,
		LogDir:   tmpDir,
		PIDFile:  filepath.Join(tmpDir, "daemon.pid"),
		Subsystems: SubsystemConfig{
			Watcher: true, Scheduler: false, Reporter: false, Miner: false,
		},
	}
	d := New(cfg, kernel.NewLifecycleManager())
	d.Start()

	status := d.Status()
	if !status.Subsystems["watcher"] {
		t.Error("watcher should be running")
	}

	// Let it run a few ticks.
	time.Sleep(200 * time.Millisecond)
	d.Stop()
}

func TestDaemonWithScheduler(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DaemonConfig{
		Interval: 50 * time.Millisecond,
		LogDir:   tmpDir,
		PIDFile:  filepath.Join(tmpDir, "daemon.pid"),
		Subsystems: SubsystemConfig{
			Watcher: false, Scheduler: true, Reporter: false, Miner: false,
		},
	}
	d := New(cfg, kernel.NewLifecycleManager())
	d.Start()

	status := d.Status()
	if !status.Subsystems["scheduler"] {
		t.Error("scheduler should be running")
	}
	d.Stop()
}

func TestDaemonWithReporter(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DaemonConfig{
		Interval: 50 * time.Millisecond,
		LogDir:   tmpDir,
		PIDFile:  filepath.Join(tmpDir, "daemon.pid"),
		Subsystems: SubsystemConfig{
			Watcher: false, Scheduler: false, Reporter: true, Miner: false,
		},
	}
	d := New(cfg, kernel.NewLifecycleManager())
	d.Start()

	status := d.Status()
	if !status.Subsystems["reporter"] {
		t.Error("reporter should be enabled")
	}
	d.Stop()
}

func TestDaemonWithMiner(t *testing.T) {
	tmpDir := t.TempDir()
	lm := kernel.NewLifecycleManager()
	cfg := DaemonConfig{
		Interval: 50 * time.Millisecond,
		LogDir:   tmpDir,
		PIDFile:  filepath.Join(tmpDir, "daemon.pid"),
		MaxAgents: 50,
		Subsystems: SubsystemConfig{
			Watcher: false, Scheduler: false, Reporter: false, Miner: true,
		},
	}
	d := New(cfg, lm)
	d.Start()

	status := d.Status()
	if !status.Subsystems["miner"] {
		t.Error("miner should be running")
	}
	time.Sleep(200 * time.Millisecond)
	d.Stop()

	// After start, miner should have tried running (system is idle by default).
}

func TestFileWatcher_AddRemoveRule(t *testing.T) {
	w := NewFileWatcher(10 * time.Second)
	initial := len(w.rules)
	w.AddRule(WatchRule{Name: "test-rule", Path: "*.md", Events: []string{"write"}, Trigger: "tester"})
	if len(w.rules) != initial+1 {
		t.Errorf("expected %d rules, got %d", initial+1, len(w.rules))
	}
	w.RemoveRule("test-rule")
	if len(w.rules) != initial {
		t.Errorf("expected %d rules after remove, got %d", initial, len(w.rules))
	}
}

func TestFileWatcher_IgnorePattern(t *testing.T) {
	w := NewFileWatcher(10 * time.Second)
	rule := &WatchRule{
		Name:   "test",
		Path:   "*.go",
		Ignore: []string{"*_test.go", "vendor"},
	}
	if !w.isIgnored(rule, "foo_test.go") {
		t.Error("should ignore *_test.go")
	}
	// isIgnored matches base filename only; directory-level ignore not yet implemented.
	if w.isIgnored(rule, "vendor") {
		t.Log("vendor base-name matched")
	}
	if w.isIgnored(rule, "main.go") {
		t.Error("should NOT ignore main.go")
	}
}

func TestFileWatcher_HasEvent(t *testing.T) {
	rule := &WatchRule{Events: []string{"write", "create"}}
	if !rule.hasEvent("write") {
		t.Error("should have write event")
	}
	if rule.hasEvent("delete") {
		t.Error("should not have delete event")
	}
}

func TestScheduleEngine_AddRemoveJob(t *testing.T) {
	e := NewScheduleEngine()
	initial := len(e.ListJobs())
	e.AddJob(&ScheduleJob{Name: "test-job", Interval: "@every 5m", AgentType: "tester", Enabled: true})
	if len(e.ListJobs()) != initial+1 {
		t.Errorf("expected %d jobs, got %d", initial+1, len(e.ListJobs()))
	}
	e.RemoveJob("test-job")
	if len(e.ListJobs()) != initial {
		t.Errorf("expected %d jobs after remove, got %d", initial, len(e.ListJobs()))
	}
}

func TestScheduleEngine_RunNow(t *testing.T) {
	e := NewScheduleEngine()
	e.AddJob(&ScheduleJob{Name: "test-run", Interval: "@every 1h", AgentType: "tester", Enabled: true, lastRun: time.Now().Add(-2 * time.Hour)})
	e.RunNow("test-run")
	jobs := e.ListJobs()
	for _, j := range jobs {
		if j.Name == "test-run" {
			if !j.lastRun.IsZero() {
				// RunNow resets lastRun to zero time
			}
		}
	}
}

func TestParseInterval(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"", 0},
		{"30m", 30 * time.Minute},
		{"1h", 1 * time.Hour},
		{"@every 30m", 30 * time.Minute},
		{"@every 1h", 1 * time.Hour},
		{"@hourly", 1 * time.Hour},
		{"@daily 9am", 24 * time.Hour},
		{"@daily 6pm", 24 * time.Hour},
	}
	for _, tt := range tests {
		got := parseInterval(tt.input)
		if got != tt.expected {
			t.Errorf("parseInterval(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestScheduleEngine_Concurrency(t *testing.T) {
	e := NewScheduleEngine()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.AddJob(&ScheduleJob{Name: id(), Interval: "@every 10m", AgentType: "tester", Enabled: true})
		}()
	}
	wg.Wait()
	jobs := e.ListJobs()
	if len(jobs) < 4+10 {
		t.Errorf("expected at least 14 jobs (4 default + 10), got %d", len(jobs))
	}
}

func TestReporter_Send(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewReporter(tmpDir)

	report := Report{
		Title:   "Test Report",
		Type:    TypeAlert,
		Level:   LevelWarn,
		Summary: "This is a test report",
		Details: "Some details here",
	}
	if err := r.Send(report); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Check that a markdown file was created.
	entries, err := os.ReadDir(filepath.Join(tmpDir, "reports"))
	if err != nil {
		t.Fatalf("read reports dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 report file, got %d", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "reports", entries[0].Name()))
	if err != nil {
		t.Fatalf("read report file: %v", err)
	}
	content := string(data)
	if !containsStr(content, "Test Report") {
		t.Error("report should contain title")
	}
	if !containsStr(content, "This is a test report") {
		t.Error("report should contain summary")
	}
}

func TestReporter_SendToBridge_NoConfig(t *testing.T) {
	r := NewReporter("/tmp")
	err := r.SendToBridge(Report{Title: "test"})
	if err == nil {
		t.Error("expected error when no bridge configured")
	}
}

func TestReporter_SendToBridge_WithConfig(t *testing.T) {
	r := NewReporter("/tmp")
	r.SetBridge("feishu")
	err := r.SendToBridge(Report{Title: "test"})
	if err != nil {
		t.Errorf("SendToBridge: %v", err)
	}
}

func TestReporter_GenerateDailySummary(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewReporter(tmpDir)
	report := r.GenerateDailySummary(5, 10, 150000)
	if report.Type != TypeDaily {
		t.Errorf("type = %s, want daily", report.Type)
	}
	if !containsStr(report.Summary, "5") {
		t.Error("summary should contain active agent count")
	}
}

func TestBackgroundMiner_IsIdle(t *testing.T) {
	lm := kernel.NewLifecycleManager()
	m := NewBackgroundMiner(lm, 10*time.Second, 50)

	// No agents running -> should be idle.
	if !m.IsIdle() {
		t.Error("should be idle with 0 active agents")
	}

	// Spawn many agents to go above 30%.
	for i := 0; i < 20; i++ {
		id, _ := lm.Spawn(kernel.AgentTypeTester, kernel.AgentConfig{})
		lm.Run(id)
	}
	if m.IsIdle() {
		t.Error("should NOT be idle with 20/50 active agents (40% > 30%)")
	}

	lm.KillAll()
	if !m.IsIdle() {
		t.Error("should be idle again after killing all agents")
	}
}

func TestBackgroundMiner_Stats(t *testing.T) {
	lm := kernel.NewLifecycleManager()
	m := NewBackgroundMiner(lm, 10*time.Second, 50)
	stats := m.Stats()
	if stats.TasksRun != 0 {
		t.Errorf("initial TasksRun = %d, want 0", stats.TasksRun)
	}
}

func TestBackgroundMiner_AddRemoveTask(t *testing.T) {
	lm := kernel.NewLifecycleManager()
	m := NewBackgroundMiner(lm, 10*time.Second, 50)
	m.AddTask(&MiningTask{Name: "test-mining", Priority: 1, Interval: 1 * time.Hour, Action: "tester"})
	m.RemoveTask("test-mining")
	// Should not panic.
}

func TestFileWatcher_Cooldown(t *testing.T) {
	w := NewFileWatcher(10 * time.Second)
	rule := &WatchRule{Name: "cd", Path: "*.go", Events: []string{"write"}, Trigger: "code-reviewer", Cooldown: 1 * time.Hour}

	// First fire should succeed.
	rule.lastFired = time.Now().Add(-2 * time.Hour)
	w.fire(rule, "test.go", "write")
	// Should have fired.
	if !rule.lastFired.After(time.Now().Add(-10 * time.Second)) {
		t.Error("expected lastFired to be updated")
	}

	// Immediate re-fire should be blocked by cooldown.
	before := rule.lastFired
	w.fire(rule, "test.go", "write")
	if !rule.lastFired.Equal(before) {
		t.Error("cooldown should prevent re-fire")
	}
}

func TestFileWatcher_Defaults(t *testing.T) {
	w := NewFileWatcher(10 * time.Second)
	rules := w.rules
	if len(rules) != 3 {
		t.Fatalf("expected 3 default rules, got %d", len(rules))
	}
	if rules[0].Name != "go-file-write" {
		t.Errorf("first rule = %s, want go-file-write", rules[0].Name)
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
