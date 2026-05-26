package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGoSearchProject(t *testing.T) string {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.22"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tmsg := greet(\"world\")\n\tfmt.Println(msg)\n}\n\nfunc greet(name string) string {\n\treturn \"hello \" + name\n}\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "lib.go"), []byte("package main\n\nfunc helper(x int) string {\n\tif x > 0 {\n\t\treturn \"positive\"\n\t}\n\treturn \"zero or negative\"\n}\n"), 0644)
	return dir
}

func setupPythonSearchProject(t *testing.T) string {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("requests==2.28.0\nflask\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "app.py"), []byte("import os\n\ndef main():\n    print('hello')\n\ndef helper():\n    return \"help\"\n\nclass App:\n    def run(self):\n        pass\n"), 0644)
	return dir
}

func setupMultiLangProject(t *testing.T) string {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.22"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\nfunc HandleRequest() {}\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "script.py"), []byte("\ndef handle_request():\n    pass\n\nclass MyClass:\n    pass\n"), 0644)
	return dir
}

func setupSearchTestServer(t *testing.T, projectRoot string) *server.MCPServer {
	srv := server.NewMCPServer("test-search", "0.1.0")
	RegisterSearchTools(srv, projectRoot)
	return srv
}

func callSearchTool(t *testing.T, srv *server.MCPServer, name string, args map[string]any) *mcp.CallToolResult {
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

func searchToolText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content, "expected non-empty content")
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent")
	return tc.Text
}

func parseSearchHits(t *testing.T, result *mcp.CallToolResult) []searchHit {
	t.Helper()
	var hits []searchHit
	require.NoError(t, json.Unmarshal([]byte(searchToolText(t, result)), &hits))
	return hits
}

func TestSearchCode(t *testing.T) {
	t.Run("finds existing string", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "search_code", map[string]any{"query": "greet"}))
		require.NotEmpty(t, hits)
		found := false
		for _, h := range hits {
			if strings.Contains(h.Snippet, "func greet") {
				found = true
				assert.Equal(t, "main.go", h.Path)
				assert.Greater(t, h.LineNumber, 0)
			}
		}
		assert.True(t, found, "expected hit for func greet")
	})
	t.Run("case-insensitive by default", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "search_code", map[string]any{"query": "PRINTLN"}))
		assert.NotEmpty(t, hits)
	})
	t.Run("case-sensitive no match", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "search_code", map[string]any{"query": "PRINTLN", "case_sensitive": true}))
		assert.Empty(t, hits)
	})
	t.Run("returns empty for nonexistent string", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "search_code", map[string]any{"query": "zzz_not_found_xyz"}))
		assert.Empty(t, hits)
	})
	t.Run("respects max_results", func(t *testing.T) {
		root := setupGoSearchProject(t)
		for i := 0; i < 5; i++ {
			_ = os.WriteFile(filepath.Join(root, "lib"+string(rune('a'+i))+".go"), []byte("package main\n\nvar markerVar int\nvar markerVar2 int\nvar markerVar3 int\n"), 0644)
		}
		srv := setupSearchTestServer(t, root)
		hits := parseSearchHits(t, callSearchTool(t, srv, "search_code", map[string]any{"query": "markerVar", "max_results": float64(3)}))
		assert.LessOrEqual(t, len(hits), 3)
		assert.NotEmpty(t, hits)
	})
	t.Run("errors on missing query", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		assert.True(t, callSearchTool(t, srv, "search_code", nil).IsError)
	})
}

func TestGrepRegex(t *testing.T) {
	t.Run("matches valid regex", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "grep_regex", map[string]any{"pattern": `func\s+\w+`}))
		require.NotEmpty(t, hits)
		found := false
		for _, h := range hits {
			if strings.Contains(h.Snippet, "func main") {
				found = true
			}
		}
		assert.True(t, found, "expected func main match")
	})
	t.Run("invalid regex returns error", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		result := callSearchTool(t, srv, "grep_regex", map[string]any{"pattern": `[unclosed`})
		assert.True(t, result.IsError)
		tc, _ := result.Content[0].(mcp.TextContent)
		assert.Contains(t, strings.ToLower(tc.Text), "invalid regex")
	})
	t.Run("no matches returns empty", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "grep_regex", map[string]any{"pattern": `zzzz_no_match_xyz`}))
		assert.Empty(t, hits)
	})
	t.Run("missing pattern errors", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		assert.True(t, callSearchTool(t, srv, "grep_regex", nil).IsError)
	})
}

func TestFindReferences(t *testing.T) {
	t.Run("finds symbol references", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "find_references", map[string]any{"symbol": "greet"}))
		require.NotEmpty(t, hits)
		for _, h := range hits {
			assert.Contains(t, h.Snippet, "greet")
			assert.NotEmpty(t, h.Context, "references should have context")
		}
	})
	t.Run("finds common symbol", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "find_references", map[string]any{"symbol": "string"}))
		assert.NotEmpty(t, hits)
	})
	t.Run("nonexistent symbol returns empty", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "find_references", map[string]any{"symbol": "nonexistentXYZ"}))
		assert.Empty(t, hits)
	})
	t.Run("missing symbol errors", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		assert.True(t, callSearchTool(t, srv, "find_references", nil).IsError)
	})
}

func TestFindDefinition(t *testing.T) {
	t.Run("finds Go func definition", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "find_definition", map[string]any{"symbol": "greet", "language": "Go"}))
		require.NotEmpty(t, hits)
		found := false
		for _, h := range hits {
			if strings.Contains(h.Snippet, "func greet") {
				found = true
				assert.Equal(t, "main.go", h.Path)
			}
		}
		assert.True(t, found)
	})
	t.Run("finds Python def definition", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupPythonSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "find_definition", map[string]any{"symbol": "main", "language": "python"}))
		require.NotEmpty(t, hits)
		found := false
		for _, h := range hits {
			if strings.Contains(h.Snippet, "def main") {
				found = true
			}
		}
		assert.True(t, found)
	})
	t.Run("finds Python class definition", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupPythonSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "find_definition", map[string]any{"symbol": "App", "language": "python"}))
		require.NotEmpty(t, hits)
		found := false
		for _, h := range hits {
			if strings.Contains(h.Snippet, "class App") {
				found = true
			}
		}
		assert.True(t, found)
	})
	t.Run("finds definition across languages", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupMultiLangProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "find_definition", map[string]any{"symbol": "HandleRequest"}))
		require.NotEmpty(t, hits)
		found := false
		for _, h := range hits {
			if strings.Contains(h.Snippet, "func HandleRequest") || strings.Contains(h.Snippet, "def handle_request") {
				found = true
			}
		}
		assert.True(t, found, "expected HandleRequest/handle_request definition")
	})
	t.Run("nonexistent symbol returns empty", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		hits := parseSearchHits(t, callSearchTool(t, srv, "find_definition", map[string]any{"symbol": "nonexistentXYZ", "language": "Go"}))
		assert.Empty(t, hits)
	})
	t.Run("missing symbol errors", func(t *testing.T) {
		srv := setupSearchTestServer(t, setupGoSearchProject(t))
		assert.True(t, callSearchTool(t, srv, "find_definition", nil).IsError)
	})
}
