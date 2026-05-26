package bridge

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages bridge registration and lifecycle.
type Registry struct {
	mu      sync.RWMutex
	bridges map[string]Bridge
}

// NewRegistry creates an empty bridge registry.
func NewRegistry() *Registry {
	return &Registry{bridges: make(map[string]Bridge)}
}

// Register adds a bridge to the registry.
func (r *Registry) Register(b Bridge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b.Name() == "" {
		return fmt.Errorf("bridge name is required")
	}
	r.bridges[b.Name()] = b
	return nil
}

// Get returns a bridge by name.
func (r *Registry) Get(name string) (Bridge, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.bridges[name]
	return b, ok
}

// All returns all registered bridges.
func (r *Registry) All() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var infos []Info
	for _, b := range r.bridges {
		infos = append(infos, Info{
			Name:   b.Name(),
			Type:   b.Type(),
			Status: b.Status(),
		})
	}
	return infos
}

// ConnectAll establishes connections for all registered bridges.
func (r *Registry) ConnectAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, b := range r.bridges {
		if err := b.Connect(ctx); err != nil {
			return fmt.Errorf("connect %s: %w", b.Name(), err)
		}
	}
	return nil
}

// DisconnectAll closes all bridge connections.
func (r *Registry) DisconnectAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, b := range r.bridges {
		b.Disconnect()
	}
	return nil
}

// Names returns the names of all registered bridges.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.bridges))
	for n := range r.bridges {
		names = append(names, n)
	}
	return names
}
