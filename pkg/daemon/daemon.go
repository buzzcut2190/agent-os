package daemon

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/agent-os/agent-os/pkg/kernel"
	"github.com/google/uuid"
)

// DaemonStatus reports the current state of the daemon and its subsystems.
type DaemonStatus struct {
	Running    bool              `json:"running"`
	StartTime  time.Time         `json:"start_time"`
	Uptime     time.Duration     `json:"uptime"`
	Subsystems map[string]bool   `json:"subsystems"`
	PID        int               `json:"pid"`
}

// Daemon is the core agent daemon that orchestrates background subsystems.
// It uses the kernel LifecycleManager to observe agent activity.
type Daemon struct {
	cfg     DaemonConfig
	kernel  *kernel.LifecycleManager
	mu      sync.RWMutex
	started time.Time
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	watcher   *FileWatcher
	scheduler *ScheduleEngine
	reporter  *Reporter
	miner     *BackgroundMiner
}

// New creates a new Daemon with the given config and kernel.
func New(cfg DaemonConfig, k *kernel.LifecycleManager) *Daemon {
	ctx, cancel := context.WithCancel(context.Background())
	return &Daemon{
		cfg:    cfg,
		kernel: k,
		ctx:    ctx,
		cancel: cancel,
	}
}

// writePID writes the current process ID to the configured PID file.
func (d *Daemon) writePID() error {
	dir := d.cfg.LogDir
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("daemon: mkdir %s: %w", dir, err)
	}
	return os.WriteFile(d.cfg.PIDFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

// removePID removes the PID file on shutdown.
func (d *Daemon) removePID() {
	os.Remove(d.cfg.PIDFile)
}

// Start launches all enabled subsystems in separate goroutines.
func (d *Daemon) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started != (time.Time{}) {
		return fmt.Errorf("daemon already started")
	}

	if err := d.writePID(); err != nil {
		return fmt.Errorf("daemon: pid file: %w", err)
	}

	d.started = time.Now()

	// Launch subsystems only if enabled.
	if d.cfg.Subsystems.Watcher {
		d.watcher = NewFileWatcher(d.cfg.Interval)
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.watcher.Start(d.ctx)
		}()
	}

	if d.cfg.Subsystems.Scheduler {
		d.scheduler = NewScheduleEngine()
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.scheduler.Start(d.ctx)
		}()
	}

	if d.cfg.Subsystems.Reporter {
		d.reporter = NewReporter(d.cfg.LogDir)
	}

	if d.cfg.Subsystems.Miner {
		d.miner = NewBackgroundMiner(d.kernel, d.cfg.Interval, d.cfg.MaxAgents)
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.miner.Start(d.ctx)
		}()
	}

	return nil
}

// Stop gracefully shuts down all subsystems by cancelling the context
// and waiting for goroutines to finish.
func (d *Daemon) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started == (time.Time{}) {
		return fmt.Errorf("daemon not running")
	}

	d.cancel()
	d.wg.Wait()
	d.removePID()
	d.started = time.Time{}
	return nil
}

// Status returns the current daemon status including subsystem states.
func (d *Daemon) Status() DaemonStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()

	running := d.started != (time.Time{})
	uptime := time.Duration(0)
	if running {
		uptime = time.Since(d.started)
	}

	subs := map[string]bool{
		"watcher":   d.cfg.Subsystems.Watcher && d.watcher != nil,
		"scheduler": d.cfg.Subsystems.Scheduler && d.scheduler != nil,
		"reporter":  d.cfg.Subsystems.Reporter && d.reporter != nil,
		"miner":     d.cfg.Subsystems.Miner && d.miner != nil,
	}

	return DaemonStatus{
		Running:    running,
		StartTime:  d.started,
		Uptime:     uptime,
		Subsystems: subs,
		PID:        os.Getpid(),
	}
}

// id generates a short unique identifier.
func id() string {
	return uuid.New().String()[:8]
}
