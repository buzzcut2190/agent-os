package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// HomeDir manages the persistent configuration, profile, and cache
// living under the home/ subdirectory of a workspace.
type HomeDir struct {
	root string
}

// NewHomeDir creates a HomeDir rooted at the given absolute path.
func NewHomeDir(root string) *HomeDir {
	return &HomeDir{root: root}
}

// configPath returns home/config.json.
func (h *HomeDir) configPath() string {
	return filepath.Join(h.root, "config.json")
}

// profilePath returns home/profile.json.
func (h *HomeDir) profilePath() string {
	return filepath.Join(h.root, "profile.json")
}

// cacheDir returns home/cache/.
func (h *HomeDir) cacheDir() string {
	return filepath.Join(h.root, "cache")
}

// GetConfig reads and returns the full config map from home/config.json.
// Returns an empty map if the file does not exist.
func (h *HomeDir) GetConfig() (map[string]any, error) {
	data, err := os.ReadFile(h.configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}
		return nil, fmt.Errorf("home: read config: %w", err)
	}
	if len(data) == 0 {
		return make(map[string]any), nil
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("home: unmarshal config: %w", err)
	}
	return cfg, nil
}

// SetConfig writes the config map to home/config.json.
func (h *HomeDir) SetConfig(cfg map[string]any) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("home: marshal config: %w", err)
	}
	if err := os.WriteFile(h.configPath(), data, 0o644); err != nil {
		return fmt.Errorf("home: write config: %w", err)
	}
	return nil
}

// GetProfile reads the agent profile from home/profile.json. A default
// profile is returned when the file does not exist.
func (h *HomeDir) GetProfile() (*AgentProfile, error) {
	data, err := os.ReadFile(h.profilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &AgentProfile{Style: StyleBalanced}, nil
		}
		return nil, fmt.Errorf("home: read profile: %w", err)
	}
	var profile AgentProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("home: unmarshal profile: %w", err)
	}
	if profile.Style == "" {
		profile.Style = StyleBalanced
	}
	return &profile, nil
}

// SetProfile writes the agent profile to home/profile.json.
func (h *HomeDir) SetProfile(profile *AgentProfile) error {
	if profile == nil {
		return fmt.Errorf("home: profile must not be nil")
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("home: marshal profile: %w", err)
	}
	if err := os.WriteFile(h.profilePath(), data, 0o644); err != nil {
		return fmt.Errorf("home: write profile: %w", err)
	}
	return nil
}

// CacheGet retrieves a cached value by key. It returns an error if the
// entry does not exist or has expired.
func (h *HomeDir) CacheGet(key string) ([]byte, error) {
	entryPath := filepath.Join(h.cacheDir(), key+".bin")
	metaPath := filepath.Join(h.cacheDir(), key+".ttl")

	data, err := os.ReadFile(entryPath)
	if err != nil {
		return nil, fmt.Errorf("home: cache get %s: %w", key, err)
	}

	// Check TTL.
	if meta, err := os.ReadFile(metaPath); err == nil {
		expiry, parseErr := strconv.ParseInt(strings.TrimSpace(string(meta)), 10, 64)
		if parseErr == nil && expiry > 0 && time.Now().Unix() > expiry {
			return nil, fmt.Errorf("home: cache get %s: expired", key)
		}
	}

	return data, nil
}

// CacheSet stores a value under the given key with an optional TTL. A
// zero TTL means the entry never expires.
func (h *HomeDir) CacheSet(key string, value []byte, ttl time.Duration) error {
	if err := os.MkdirAll(h.cacheDir(), 0o755); err != nil {
		return fmt.Errorf("home: cache set: %w", err)
	}

	entryPath := filepath.Join(h.cacheDir(), key+".bin")
	if err := os.WriteFile(entryPath, value, 0o644); err != nil {
		return fmt.Errorf("home: cache set %s: %w", key, err)
	}

	metaPath := filepath.Join(h.cacheDir(), key+".ttl")
	var expiry int64
	if ttl > 0 {
		expiry = time.Now().Add(ttl).Unix()
	}
	if err := os.WriteFile(metaPath, []byte(strconv.FormatInt(expiry, 10)), 0o644); err != nil {
		return fmt.Errorf("home: cache set %s ttl: %w", key, err)
	}

	return nil
}

// CacheClear removes all entries from the cache directory.
func (h *HomeDir) CacheClear() error {
	entries, err := os.ReadDir(h.cacheDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("home: cache clear: %w", err)
	}
	for _, entry := range entries {
		path := filepath.Join(h.cacheDir(), entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("home: cache clear %s: %w", entry.Name(), err)
		}
	}
	return nil
}
