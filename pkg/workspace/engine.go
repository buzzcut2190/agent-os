package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Engine is the central manager for agent workspaces. It creates,
// persists, and retrieves workspace instances, each backed by physical
// directories under basePath.
type Engine struct {
	basePath   string
	workspaces map[WorkspaceID]*Workspace
	mu         sync.RWMutex
}

// NewEngine creates an Engine that stores workspace data under basePath.
// The base directory is created if it does not exist.
func NewEngine(basePath string) (*Engine, error) {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("workspace engine: create base path: %w", err)
	}

	e := &Engine{
		basePath:   basePath,
		workspaces: make(map[WorkspaceID]*Workspace),
	}

	if err := e.loadRegistry(); err != nil {
		return nil, fmt.Errorf("workspace engine: load registry: %w", err)
	}

	return e, nil
}

// generateID returns an 8-character hex identifier derived from a UUID.
func generateID() WorkspaceID {
	id := uuid.New().String()
	id = strings.ReplaceAll(id, "-", "")
	return WorkspaceID(id[:8])
}

// registryPath returns the filesystem path to the workspace registry.
func (e *Engine) registryPath() string {
	return filepath.Join(e.basePath, "registry.json")
}

// subDirs returns the subdirectory names created under each workspace.
func subDirs() []string {
	return []string{"home", "scratch", "artifacts", "memory"}
}

// Create provisions a new workspace on disk with the given owner, type,
// and configuration. An 8‑character workspace ID is generated
// automatically. The method creates the workspace root and its
// subdirectories (home/, scratch/, artifacts/, memory/), then persists
// the registry.
func (e *Engine) Create(ownerID string, wsType WorkspaceType, config WorkspaceConfig) (*Workspace, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	now := time.Now()
	ws := &Workspace{
		ID:        generateID(),
		OwnerID:   ownerID,
		Type:      wsType,
		CreatedAt: now,
		UpdatedAt: now,
		Status:    StatusActive,
		Storage: WorkspaceStorage{
			LimitBytes: 0, // unlimited by default
		},
		Config: config,
	}

	root := e.wsPath(ws.ID)
	for _, sub := range subDirs() {
		dir := filepath.Join(root, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("workspace engine: create dir %s: %w", dir, err)
		}
		// Create child dirs specific to each sub-area.
		switch sub {
		case "home":
			_ = os.MkdirAll(filepath.Join(dir, "cache"), 0o755)
		case "memory":
			_ = os.MkdirAll(filepath.Join(dir, "sessions"), 0o755)
		}
	}

	e.workspaces[ws.ID] = ws
	if err := e.saveRegistry(); err != nil {
		return nil, fmt.Errorf("workspace engine: save registry: %w", err)
	}

	return ws, nil
}

// Get returns a workspace by ID. If the workspace does not exist or has
// been deleted, an error is returned.
func (e *Engine) Get(id WorkspaceID) (*Workspace, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ws, ok := e.workspaces[id]
	if !ok {
		return nil, fmt.Errorf("workspace %s: not found", id)
	}
	if ws.Status == StatusDeleted {
		return nil, fmt.Errorf("workspace %s: deleted", id)
	}
	cp := *ws
	cp.Config.Labels = copyMap(ws.Config.Labels)
	return &cp, nil
}

// List returns copies of all workspaces that have not been deleted.
func (e *Engine) List() []*Workspace {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*Workspace
	for _, ws := range e.workspaces {
		if ws.Status == StatusDeleted {
			continue
		}
		cp := *ws
		cp.Config.Labels = copyMap(ws.Config.Labels)
		result = append(result, &cp)
	}
	return result
}

// Archive marks a workspace as archived. Archived workspaces are still
// queryable but cannot accept new writes through normal paths.
func (e *Engine) Archive(id WorkspaceID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ws, ok := e.workspaces[id]
	if !ok {
		return fmt.Errorf("workspace %s: not found", id)
	}
	ws.Status = StatusArchived
	ws.UpdatedAt = time.Now()
	return e.saveRegistry()
}

// Delete marks a workspace as deleted. Physical directories are NOT
// removed so that operators can recover data if needed.
func (e *Engine) Delete(id WorkspaceID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ws, ok := e.workspaces[id]
	if !ok {
		return fmt.Errorf("workspace %s: not found", id)
	}
	ws.Status = StatusDeleted
	ws.UpdatedAt = time.Now()
	return e.saveRegistry()
}

// GetByOwner returns all non‑deleted workspaces owned by the given
// owner ID.
func (e *Engine) GetByOwner(ownerID string) []*Workspace {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*Workspace
	for _, ws := range e.workspaces {
		if ws.OwnerID == ownerID && ws.Status != StatusDeleted {
			cp := *ws
			cp.Config.Labels = copyMap(ws.Config.Labels)
			result = append(result, &cp)
		}
	}
	return result
}

// wsPath returns the absolute filesystem path for a workspace ID.
func (e *Engine) wsPath(id WorkspaceID) string {
	return filepath.Join(e.basePath, string(id))
}

// HomePath returns the absolute path to the home directory of the
// workspace.
func (e *Engine) HomePath(id WorkspaceID) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.workspaces[id]; !ok {
		return "", fmt.Errorf("workspace %s: not found", id)
	}
	return filepath.Join(e.wsPath(id), "home"), nil
}

// ScratchPath returns the absolute path to the scratch directory.
func (e *Engine) ScratchPath(id WorkspaceID) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.workspaces[id]; !ok {
		return "", fmt.Errorf("workspace %s: not found", id)
	}
	return filepath.Join(e.wsPath(id), "scratch"), nil
}

// ArtifactsPath returns the absolute path to the artifacts directory.
func (e *Engine) ArtifactsPath(id WorkspaceID) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.workspaces[id]; !ok {
		return "", fmt.Errorf("workspace %s: not found", id)
	}
	return filepath.Join(e.wsPath(id), "artifacts"), nil
}

// MemoryPath returns the absolute path to the memory directory.
func (e *Engine) MemoryPath(id WorkspaceID) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.workspaces[id]; !ok {
		return "", fmt.Errorf("workspace %s: not found", id)
	}
	return filepath.Join(e.wsPath(id), "memory"), nil
}

// saveRegistry persists the in‑memory workspace map to registry.json.
func (e *Engine) saveRegistry() error {
	data, err := json.MarshalIndent(e.workspaces, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	if err := os.WriteFile(e.registryPath(), data, 0o644); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}
	return nil
}

// loadRegistry reads registry.json into the in‑memory workspace map.
func (e *Engine) loadRegistry() error {
	path := e.registryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No registry yet — first run.
			return nil
		}
		return fmt.Errorf("read registry: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &e.workspaces); err != nil {
		return fmt.Errorf("unmarshal registry: %w", err)
	}
	return nil
}

func copyMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
