package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/agent-os/agent-os/pkg/context"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGoProject(t *testing.T) string {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.22"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "lib.go"), []byte("package main\n\nfunc helper() string {\n\treturn \"\"\n}\n"), 0644)
	return dir
}

func setupTypeScriptProject(t *testing.T) string {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test","version":"1.0.0"}`), 0644)
	_ = os.WriteFile(filepath.Join(dir, "index.ts"), []byte("const x: number = 1;\nconsole.log(x);\n"), 0644)
	return dir
}

func setupContextTestServer(t *testing.T, projectRoot string) *server.MCPServer {
	srv := server.NewMCPServer("test-context", "0.1.0")
	RegisterContextTools(srv, projectRoot)
	return srv
}

func callTool(t *testing.T, srv *server.MCPServer, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	st := srv.ListTools()[name]
	require.NotNil(t, st, "tool %q not registered", name)
	result, err := st.Handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: name, Arguments: args},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

func toolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content, "expected non-empty content")
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent")
	return tc.Text
}

func TestGetContext(t *testing.T) {
	t.Run("generates summary on Go project", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		text := toolResultText(t, callTool(t, srv, "get_context", nil))
		var s pkgctx.ProjectSummary
		require.NoError(t, json.Unmarshal([]byte(text), &s))
		assert.NotEmpty(t, s.ProjectName)
		assert.NotEmpty(t, s.DirectoryTree)
		assert.Greater(t, s.TotalFiles, 0)
		assert.NotNil(t, s.FileStatistics)
	})
	t.Run("returns cached result by default", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		r1 := toolResultText(t, callTool(t, srv, "get_context", nil))
		r2 := toolResultText(t, callTool(t, srv, "get_context", nil))
		assert.Equal(t, r1, r2)
	})
	t.Run("force_refresh invalidates cache", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		assert.NotEmpty(t, toolResultText(t, callTool(t, srv, "get_context", map[string]any{"force_refresh": true})))
		assert.NotEmpty(t, toolResultText(t, callTool(t, srv, "get_context", nil)))
	})
	t.Run("empty project returns valid summary", func(t *testing.T) {
		srv := setupContextTestServer(t, t.TempDir())
		text := toolResultText(t, callTool(t, srv, "get_context", nil))
		var s pkgctx.ProjectSummary
		require.NoError(t, json.Unmarshal([]byte(text), &s))
		assert.Equal(t, 0, s.TotalFiles)
		assert.NotNil(t, s.FileStatistics)
	})
}

func TestGetFileSummary(t *testing.T) {
	t.Run("returns metadata for existing file", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		text := toolResultText(t, callTool(t, srv, "get_file_summary", map[string]any{"path": "main.go"}))
		var fd fileDesc
		require.NoError(t, json.Unmarshal([]byte(text), &fd))
		assert.Contains(t, fd.Path, "main.go")
		assert.Greater(t, fd.Size, int64(0))
		assert.Greater(t, fd.Lines, 0)
		assert.NotEmpty(t, fd.Mtime)
	})
	t.Run("errors on nonexistent file", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		result := callTool(t, srv, "get_file_summary", map[string]any{"path": "nonexistent.go"})
		assert.True(t, result.IsError)
	})
	t.Run("rejects path traversal", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		result := callTool(t, srv, "get_file_summary", map[string]any{"path": "../etc/passwd"})
		assert.True(t, result.IsError)
		tc, _ := result.Content[0].(mcp.TextContent)
		assert.Contains(t, strings.ToLower(tc.Text), "invalid path")
	})
	t.Run("rejects absolute path outside root", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		result := callTool(t, srv, "get_file_summary", map[string]any{"path": "/etc/passwd"})
		assert.True(t, result.IsError)
	})
}

func TestDetectStack(t *testing.T) {
	t.Run("Go project via go.mod", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		text := toolResultText(t, callTool(t, srv, "detect_stack", nil))
		var types []pkgctx.ProjectType
		require.NoError(t, json.Unmarshal([]byte(text), &types))
		require.NotEmpty(t, types)
		found := false
		for _, pt := range types {
			if pt.Language == "Go" {
				found = true
				assert.Equal(t, "Go Modules", pt.BuildSystem)
			}
		}
		assert.True(t, found, "Go language not detected")
	})
	t.Run("TypeScript project via package.json", func(t *testing.T) {
		srv := setupContextTestServer(t, setupTypeScriptProject(t))
		text := toolResultText(t, callTool(t, srv, "detect_stack", nil))
		var types []pkgctx.ProjectType
		require.NoError(t, json.Unmarshal([]byte(text), &types))
		require.NotEmpty(t, types)
		found := false
		for _, pt := range types {
			if pt.Language == "TypeScript" {
				found = true
			}
		}
		assert.True(t, found, "TypeScript language not detected")
	})
	t.Run("empty project returns empty slice", func(t *testing.T) {
		srv := setupContextTestServer(t, t.TempDir())
		text := toolResultText(t, callTool(t, srv, "detect_stack", nil))
		var types []pkgctx.ProjectType
		require.NoError(t, json.Unmarshal([]byte(text), &types))
		assert.Empty(t, types)
	})
}

func TestCountLOC(t *testing.T) {
	t.Run("counts files and lines", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		text := toolResultText(t, callTool(t, srv, "count_loc", nil))
		var lr locResult
		require.NoError(t, json.Unmarshal([]byte(text), &lr))
		assert.Greater(t, lr.TotalFiles, 0)
		assert.Greater(t, lr.TotalLines, 0)
		if stat, ok := lr.ByLanguage["Go"]; ok {
			assert.Greater(t, stat.Files, 0)
			assert.Greater(t, stat.Lines, 0)
		}
	})
	t.Run("filters by language", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		text := toolResultText(t, callTool(t, srv, "count_loc", map[string]any{"language": "Go"}))
		var lr locResult
		require.NoError(t, json.Unmarshal([]byte(text), &lr))
		assert.Greater(t, lr.TotalFiles, 0)
		for lang := range lr.ByLanguage {
			assert.Equal(t, "Go", lang)
		}
	})
	t.Run("nonexistent language yields zero", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		text := toolResultText(t, callTool(t, srv, "count_loc", map[string]any{"language": "Rust"}))
		var lr locResult
		require.NoError(t, json.Unmarshal([]byte(text), &lr))
		assert.Equal(t, 0, lr.TotalFiles)
		assert.Equal(t, 0, lr.TotalLines)
	})
	t.Run("empty project returns zero", func(t *testing.T) {
		srv := setupContextTestServer(t, t.TempDir())
		text := toolResultText(t, callTool(t, srv, "count_loc", nil))
		var lr locResult
		require.NoError(t, json.Unmarshal([]byte(text), &lr))
		assert.Equal(t, 0, lr.TotalFiles)
		assert.Equal(t, 0, lr.TotalLines)
		assert.Empty(t, lr.ByLanguage)
	})
	t.Run("rejects path traversal", func(t *testing.T) {
		srv := setupContextTestServer(t, setupGoProject(t))
		result := callTool(t, srv, "count_loc", map[string]any{"path": "../etc"})
		assert.True(t, result.IsError)
	})
}
