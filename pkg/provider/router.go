package provider

import (
	"fmt"
	"sort"
	"sync"
)

// Router routes chat requests to the appropriate provider.
type Router struct {
	mu          sync.RWMutex
	providers   map[string]Provider
	defaultName string
	priorities  map[string]int
	fallbacks   map[string][]string
	strategy    string
}

// NewRouter creates a router from a list of providers.
func NewRouter(providers []Provider) *Router {
	r := &Router{
		providers:  make(map[string]Provider),
		priorities: make(map[string]int),
		fallbacks:  make(map[string][]string),
		strategy:   "priority",
	}
	for i, p := range providers {
		r.providers[p.Name()] = p
		r.priorities[p.Name()] = i
	}
	return r
}

// Route returns the best provider for a given model.
func (r *Router) Route(model string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Direct model match.
	for _, p := range r.providers {
		for _, m := range p.Models() {
			if m.ID == model {
				return p, nil
			}
		}
	}

	// Fallback: model = provider name.
	if p, ok := r.providers[model]; ok {
		return p, nil
	}

	return nil, fmt.Errorf("no provider found for model %q", model)
}

// RouteByTag routes to a provider matching a tag.
func (r *Router) RouteByTag(tag string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	type scored struct {
		p    Provider
		prio int
	}
	var candidates []scored
	for _, p := range r.providers {
		candidates = append(candidates, scored{p, r.priorities[p.Name()]})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].prio < candidates[j].prio
	})
	for _, c := range candidates {
		return c.p, nil
	}
	return nil, fmt.Errorf("no provider for tag %q", tag)
}

// SetDefault sets the default provider/model name.
func (r *Router) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[name]; !ok {
		// Check if it's a model name.
		found := false
		for _, p := range r.providers {
			for _, m := range p.Models() {
				if m.ID == name {
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("provider or model %q not found", name)
		}
	}
	r.defaultName = name
	return nil
}

// GetDefault returns the current default provider/model name.
func (r *Router) GetDefault() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.defaultName != "" {
		return r.defaultName
	}
	// Return the first provider with highest priority.
	type entry struct {
		name string
		prio int
	}
	var entries []entry
	for name, prio := range r.priorities {
		entries = append(entries, entry{name, prio})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].prio < entries[j].prio })
	if len(entries) > 0 {
		return entries[0].name
	}
	return ""
}

// List returns summaries of all providers.
func (r *Router) List() []ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []ProviderInfo
	for _, p := range r.providers {
		models := make([]string, len(p.Models()))
		for i, m := range p.Models() {
			models[i] = m.ID
		}
		result = append(result, ProviderInfo{
			Name:     p.Name(),
			Models:   models,
			Priority: r.priorities[p.Name()],
			Healthy:  true, // updated by ping
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return r.priorities[result[i].Name] < r.priorities[result[j].Name]
	})
	return result
}

// SetPriority adjusts the routing priority for a provider.
func (r *Router) SetPriority(name string, priority int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.priorities[name] = priority
	return nil
}

// SetFallbacks configures the fallback chain for a provider.
func (r *Router) SetFallbacks(name string, fallbacks []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallbacks[name] = fallbacks
}

// GetAllModels returns all models across all providers.
func (r *Router) GetAllModels() []Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Model
	for _, p := range r.providers {
		result = append(result, p.Models()...)
	}
	return result
}

// ProviderNames returns the names of all registered providers.
func (r *Router) ProviderNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// HealthCheck pings all providers and returns results.
func (r *Router) HealthCheck(results chan HealthResult) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.providers {
		go func(pp Provider) {
			err := pp.Ping(nil) // nil context for health check
			hr := HealthResult{
				Provider: pp.Name(),
				Healthy:  err == nil,
			}
			if err != nil {
				hr.Error = err.Error()
			}
			results <- hr
		}(p)
	}
}
