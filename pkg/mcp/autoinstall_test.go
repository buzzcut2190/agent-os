package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chdirTemp(t *testing.T) {
	t.Helper()
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
}

func readSettings(t *testing.T, path string) *AgentConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	cfg := &AgentConfig{}
	require.NoError(t, json.Unmarshal(data, cfg))
	return cfg
}

// 1. TestInstallClaude — install creates .claude/settings.json with agentfs entry.
func TestInstallClaude(t *testing.T) {
	chdirTemp(t)
	binary := "/usr/local/bin/agentfs-mcp"

	require.NoError(t, InstallAgent("claude", false, binary))

	cfg := readSettings(t, ".claude/settings.json")
	require.Contains(t, cfg.MCPServers, "agentfs")
	srv := cfg.MCPServers["agentfs"]
	assert.Equal(t, binary, srv.Command)
	assert.Equal(t, []string{"serve", "--transport", "stdio"}, srv.Args)
	assert.Equal(t, ".", srv.Env["AGENTFS_PROJECT_ROOT"])
}

// 2. TestInstallIdempotent — second install skips, file unchanged.
func TestInstallIdempotent(t *testing.T) {
	chdirTemp(t)
	path := ".claude/settings.json"

	require.NoError(t, InstallAgent("claude", false, "/opt/bin/agentfs"))
	first, err := os.ReadFile(path)
	require.NoError(t, err)

	require.NoError(t, InstallAgent("claude", false, "/other/path"))
	second, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

// 3. TestUninstall — install then uninstall removes agentfs.
func TestUninstall(t *testing.T) {
	chdirTemp(t)
	path := ".claude/settings.json"

	require.NoError(t, InstallAgent("claude", false, "/tmp/agentfs"))
	require.Contains(t, readSettings(t, path).MCPServers, "agentfs")

	require.NoError(t, UninstallAgent("claude", false))
	assert.NotContains(t, readSettings(t, path).MCPServers, "agentfs")
}

// 4. TestUninstallMissing — uninstalling never-installed agent is a no-op.
func TestUninstallMissing(t *testing.T) {
	chdirTemp(t)
	require.NoError(t, UninstallAgent("claude", false))
	_, err := os.Stat(".claude/settings.json")
	assert.True(t, os.IsNotExist(err))
}

// 5. TestInstallInvalidAgent — unsupported agent returns error.
func TestInstallInvalidAgent(t *testing.T) {
	chdirTemp(t)
	err := InstallAgent("unknown", false, "/bin/true")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported agent")
}

// 6. TestLoadOrCreate — nil map, empty file, invalid JSON, valid JSON.
func TestLoadOrCreate(t *testing.T) {
	t.Run("nonexistent file", func(t *testing.T) {
		cfg := loadOrCreateConfig("/tmp/no-such-file.json")
		require.NotNil(t, cfg)
		assert.Empty(t, cfg.MCPServers)
	})

	t.Run("empty file", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "e.json")
		require.NoError(t, os.WriteFile(p, nil, 0644))
		cfg := loadOrCreateConfig(p)
		require.NotNil(t, cfg)
		assert.Empty(t, cfg.MCPServers)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "b.json")
		require.NoError(t, os.WriteFile(p, []byte("---"), 0644))
		cfg := loadOrCreateConfig(p)
		require.NotNil(t, cfg)
		assert.Empty(t, cfg.MCPServers)
	})

	t.Run("valid JSON", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "g.json")
		require.NoError(t, os.WriteFile(p, []byte(
			`{"mcpServers":{"x":{"command":"c","args":["a"]}}}`, ), 0644))
		cfg := loadOrCreateConfig(p)
		require.Contains(t, cfg.MCPServers, "x")
		assert.Equal(t, "c", cfg.MCPServers["x"].Command)
	})

	t.Run("nil mcpServers", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "n.json")
		require.NoError(t, os.WriteFile(p, []byte(`{}`), 0644))
		cfg := loadOrCreateConfig(p)
		require.NotNil(t, cfg.MCPServers)
		assert.Empty(t, cfg.MCPServers)
	})
}

// 7. TestConfigRoundtrip — load, mutate, write, reload, verify.
func TestConfigRoundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")

	cfg := loadOrCreateConfig(p)
	cfg.MCPServers["agentfs"] = MCPConfig{
		Command: "/bin/agentfs-mcp",
		Args:    []string{"serve", "--transport", "stdio"},
		Env:     map[string]string{"AGENTFS_PROJECT_ROOT": "/home/u/proj"},
	}
	cfg.MCPServers["other"] = MCPConfig{Command: "/bin/ls", Args: []string{"-la"}}
	require.NoError(t, writeConfig(p, cfg))

	got := loadOrCreateConfig(p)
	require.Contains(t, got.MCPServers, "agentfs")
	require.Contains(t, got.MCPServers, "other")

	assert.Equal(t, "/bin/agentfs-mcp", got.MCPServers["agentfs"].Command)
	assert.Equal(t, []string{"serve", "--transport", "stdio"}, got.MCPServers["agentfs"].Args)
	assert.Equal(t, "/home/u/proj", got.MCPServers["agentfs"].Env["AGENTFS_PROJECT_ROOT"])
	assert.Equal(t, "/bin/ls", got.MCPServers["other"].Command)
	assert.Equal(t, []string{"-la"}, got.MCPServers["other"].Args)
}
