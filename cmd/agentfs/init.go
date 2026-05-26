package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AgentFSConfig is the top-level .agentfs.yaml configuration.
type AgentFSConfig struct {
	Version      string         `yaml:"version"`
	Project      ProjectConfig  `yaml:"project"`
	Context      ContextConfig  `yaml:"context"`
	Integrations []Integration  `yaml:"integrations"`
}

type ProjectConfig struct {
	Name      string `yaml:"name"`
	MountPoint string `yaml:"mount_point"`
	AutoMount bool   `yaml:"auto_mount"`
	Ephemeral bool   `yaml:"ephemeral"`
}

type ContextConfig struct {
	MaxDepth    int      `yaml:"max_depth"`
	Exclude     []string `yaml:"exclude"`
	AutoRefresh int      `yaml:"auto_refresh"`
}

type Integration struct {
	Agent          string `yaml:"agent"`
	AutoConfigure  bool   `yaml:"auto_configure"`
}

func defaultConfig(projectName string) *AgentFSConfig {
	return &AgentFSConfig{
		Version: "1",
		Project: ProjectConfig{
			Name:       projectName,
			MountPoint: ".agentfs/mnt",
			AutoMount:  true,
			Ephemeral:  false,
		},
		Context: ContextConfig{
			MaxDepth:    3,
			Exclude:     []string{"*.log", "node_modules/"},
			AutoRefresh: 30,
		},
		Integrations: []Integration{
			{Agent: "claude", AutoConfigure: true},
		},
	}
}

func runInit(projectDir string) error {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	configPath := filepath.Join(abs, ".agentfs.yaml")

	// Idempotent: do not overwrite existing config
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Configuration already exists at %s (skipping)\n", configPath)
		return nil
	}

	cfg := defaultConfig(filepath.Base(abs))
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	return nil
}

func runIntegrate(projectDir, agent string) error {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	switch agent {
	case "claude":
		return integrateClaude(abs)
	default:
		return fmt.Errorf("unknown agent: %s (supported: claude)", agent)
	}
}

func integrateClaude(projectDir string) error {
	claudePath := filepath.Join(projectDir, "CLAUDE.md")

	block := `
## agentfs Integration

This project uses agentfs for native file system access. The agent workspace
is available at the mount point configured in .agentfs.yaml.

- Use the mount path from .agentfs.yaml for all file operations.
- Read @context at the root of the mount for a project overview.
- Changes in ephemeral mode are discarded on unmount.
`

	// Read existing content
	var content []byte
	existing, err := os.ReadFile(claudePath)
	if err == nil {
		content = existing
	}

	// Check if already injected
	if contains(string(content), "agentfs Integration") {
		fmt.Printf("agentfs already configured in %s (skipping)\n", claudePath)
		return nil
	}

	content = append(content, []byte(block)...)
	if err := os.WriteFile(claudePath, content, 0644); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}

	fmt.Printf("Updated %s with agentfs instructions\n", claudePath)
	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
