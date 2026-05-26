package mcp

import (
	"os"
	"path/filepath"
)

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	cwd, _ := os.Getwd()
	return &Config{
		ProjectRoot: cwd,
		Transport:   "stdio",
		Port:        8080,
		SessionDir:  defaultSessionDir(),
		LogLevel:    "info",
	}
}

func defaultSessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "agentfs-sessions")
	}
	return filepath.Join(home, ".local", "share", "agentfs", "sessions")
}
