package plugin

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages the plugin lifecycle: registration, lookup,
// enable/disable toggling, and unloading.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
	info    map[string]PluginInfo
}

// NewRegistry returns an initialized, empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
		info:    make(map[string]PluginInfo),
	}
}

// Register adds a plugin to the registry. It calls p.Init() with the
// provided config and records the PluginInfo. Returns an error if a
// plugin with the same name is already registered or if Init fails.
func (r *Registry) Register(p Plugin, info PluginInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[p.Name()]; exists {
		return fmt.Errorf("plugin %q is already registered", p.Name())
	}

	if err := p.Init(info.Config); err != nil {
		return fmt.Errorf("plugin %q init failed: %w", p.Name(), err)
	}

	info.Enabled = true
	r.plugins[p.Name()] = p
	r.info[p.Name()] = info
	return nil
}

// Unregister removes a plugin by name and calls its OnClose hook.
// Returns an error if the plugin is not found.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}

	// OnClose is best-effort; log but don't fail.
	_ = p.OnClose(context.TODO())

	delete(r.plugins, name)
	delete(r.info, name)
	return nil
}

// Get returns the Plugin and true if the named plugin is registered.
func (r *Registry) Get(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	return p, ok
}

// List returns metadata for all registered plugins.
func (r *Registry) List() []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]PluginInfo, 0, len(r.info))
	for _, v := range r.info {
		out = append(out, v)
	}
	return out
}

// Enable marks a plugin as active so its hooks are invoked.
func (r *Registry) Enable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inf, ok := r.info[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}
	inf.Enabled = true
	r.info[name] = inf
	return nil
}

// Disable marks a plugin as inactive; its hooks will be skipped.
func (r *Registry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inf, ok := r.info[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}
	inf.Enabled = false
	r.info[name] = inf
	return nil
}
