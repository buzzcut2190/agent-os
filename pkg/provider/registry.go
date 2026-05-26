package provider

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// Registry manages provider registration and configuration.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry.
func (r *Registry) Register(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.Name() == "" {
		return fmt.Errorf("provider name is required")
	}
	r.providers[p.Name()] = p
	return nil
}

// Get returns a registered provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// Unregister removes a provider from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, name)
}

// All returns all registered providers.
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

// LoadConfig reads providers.yaml and registers configured providers.
// It returns the parsed ConfigFile for further use (router setup, etc.).
func (r *Registry) LoadConfig(path string) (*ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg ConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	for _, pc := range cfg.Providers {
		if pc.Disabled {
			continue
		}
		p, err := NewProviderFromConfig(pc)
		if err != nil {
			continue // graceful degradation
		}
		r.Register(p)
	}
	return &cfg, nil
}

// SaveConfig writes the current provider configurations to a YAML file.
func (r *Registry) SaveConfig(path string, cfg *ConfigFile) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// NewProviderFromConfig creates a Provider from a ProviderConfig.
func NewProviderFromConfig(pc ProviderConfig) (Provider, error) {
	switch pc.Type {
	case "anthropic":
		return NewAnthropicProvider(pc), nil
	case "openai-compatible":
		return NewOpenAICompatibleProvider(pc), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", pc.Type)
	}
}
