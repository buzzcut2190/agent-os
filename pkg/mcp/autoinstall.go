package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MCPConfig is the MCP server entry in an agent's configuration.
type MCPConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// AgentConfig represents a single agent's MCP server configuration.
type AgentConfig struct {
	MCPServers map[string]MCPConfig `json:"mcpServers"`
}

// InstallAgent installs agentfs MCP configuration for the given agent.
// Returns an error if the binary path cannot be determined.
func InstallAgent(agent string, global bool, binaryPath string) error {
	if binaryPath == "" {
		var err error
		binaryPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine binary path: %w", err)
		}
	}

	switch agent {
	case "claude":
		return installClaudeAgent(binaryPath, global)
	case "cursor":
		return installCursorAgent(binaryPath, global)
	default:
		return fmt.Errorf("unsupported agent: %s (supported: claude, cursor)", agent)
	}
}

// UninstallAgent removes agentfs MCP configuration for the given agent.
func UninstallAgent(agent string, global bool) error {
	switch agent {
	case "claude":
		return uninstallClaudeAgent(global)
	default:
		return fmt.Errorf("unsupported agent: %s (supported: claude)", agent)
	}
}

func installClaudeAgent(binaryPath string, global bool) error {
	configPath := claudeConfigPath(global)
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	cfg := loadOrCreateConfig(configPath)
	if _, exists := cfg.MCPServers["agentfs"]; exists {
		fmt.Printf("agentfs already configured in %s (skipping)\n", configPath)
		return nil
	}

	cfg.MCPServers["agentfs"] = MCPConfig{
		Command: binaryPath,
		Args:    []string{"serve", "--transport", "stdio"},
		Env: map[string]string{
			"AGENTFS_PROJECT_ROOT": ".",
		},
	}

	if err := writeConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Printf("Installed agentfs MCP config for Claude at %s\n", configPath)
	return nil
}

func installCursorAgent(binaryPath string, global bool) error {
	configPath := cursorConfigPath(global)
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	cfg := loadOrCreateConfig(configPath)
	if _, exists := cfg.MCPServers["agentfs"]; exists {
		fmt.Printf("agentfs already configured in %s (skipping)\n", configPath)
		return nil
	}

	cfg.MCPServers["agentfs"] = MCPConfig{
		Command: binaryPath,
		Args:    []string{"serve", "--transport", "stdio"},
	}

	if err := writeConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Printf("Installed agentfs MCP config for Cursor at %s\n", configPath)
	return nil
}

func uninstallClaudeAgent(global bool) error {
	configPath := claudeConfigPath(global)
	cfg := loadOrCreateConfig(configPath)
	if _, exists := cfg.MCPServers["agentfs"]; !exists {
		fmt.Printf("agentfs not found in %s (skipping)\n", configPath)
		return nil
	}

	delete(cfg.MCPServers, "agentfs")
	if err := writeConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Printf("Removed agentfs MCP config from %s\n", configPath)
	return nil
}

func claudeConfigPath(global bool) string {
	if global {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".claude", "settings.json")
	}
	return filepath.Join(".claude", "settings.json")
}

func cursorConfigPath(global bool) string {
	if global {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".cursor", "mcp.json")
	}
	return filepath.Join(".cursor", "mcp.json")
}

func loadOrCreateConfig(path string) *AgentConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return &AgentConfig{MCPServers: make(map[string]MCPConfig)}
	}
	cfg := &AgentConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return &AgentConfig{MCPServers: make(map[string]MCPConfig)}
	}
	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]MCPConfig)
	}
	return cfg
}

func writeConfig(path string, cfg *AgentConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
