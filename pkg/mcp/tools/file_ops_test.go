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

func tmp(t *testing.T) string {
	d := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(d, "hello.txt"), []byte("hello world"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(d, "sub"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(d, "sub", "nested.txt"), []byte("nested content"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(d, ".hidden_dir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(d, ".hidden"), []byte("hidden"), 0644))
	return d
}

func srv(t *testing.T, root string) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	RegisterFileOps(s, root)
	return s
}

func tc(t *testing.T, s *server.MCPServer, name string, args map[string]any) *mcp.CallToolResult {
	st := s.GetTool(name)
	require.NotNil(t, st, "tool %q not registered", name)
	r, err := st.Handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: name, Arguments: args},
	})
	require.NoError(t, err)
	return r
}

func rtext(r *mcp.CallToolResult) string {
	tc, _ := r.Content[0].(mcp.TextContent)
	return tc.Text
}

func errOK(r *mcp.CallToolResult) bool { return r != nil && r.IsError }

// =============================================================================
// safePath
// =============================================================================

func TestSafePath(t *testing.T) {
	root := "/tmp/testroot"
	p, err := safePath(root, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "hello.txt"), p)

	_, err = safePath(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty path")

	p, err = safePath(root, filepath.Join(root, "sub/nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "sub", "nested.txt"), p)

	_, err = safePath(root, "/etc/passwd")
	assert.ErrorContains(t, err, "path escapes project root")

	_, err = safePath(root, "../../../etc/passwd")
	assert.ErrorContains(t, err, "path escapes project root")

	_, err = safePath(root, "foo/../../bar/../../../etc/passwd")
	assert.ErrorContains(t, err, "path escapes project root")

	p, err = safePath(root, "./hello.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "hello.txt"), p)
}

// =============================================================================
// read_file
// =============================================================================

func TestReadFile(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "read_file", map[string]any{"path": "hello.txt"})
		assert.False(t, errOK(r))
		assert.Equal(t, "hello world", rtext(r))
	})
	t.Run("offset-limit", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "read_file", map[string]any{"path": "hello.txt", "offset": float64(6), "limit": float64(5)})
		assert.Equal(t, "world", rtext(r))
	})
	t.Run("offset-past-end", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "read_file", map[string]any{"path": "hello.txt", "offset": float64(999)})
		assert.Equal(t, "", rtext(r))
	})
	t.Run("file-not-found", func(t *testing.T) {
		d := t.TempDir()
		r := tc(t, srv(t, d), "read_file", map[string]any{"path": "nope.txt"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "read:")
	})
	t.Run("traversal-rejected", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "read_file", map[string]any{"path": "../../../etc/passwd"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "invalid path")
	})
	t.Run("empty-path", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "read_file", map[string]any{"path": ""})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "path is required")
	})
}

// =============================================================================
// write_file
// =============================================================================

func TestWriteFile(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		d := t.TempDir()
		r := tc(t, srv(t, d), "write_file", map[string]any{"path": "new.txt", "content": "hello from test"})
		assert.False(t, errOK(r))
		got, _ := os.ReadFile(filepath.Join(d, "new.txt"))
		assert.Equal(t, "hello from test", string(got))
	})
	t.Run("overwrite", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "write_file", map[string]any{"path": "hello.txt", "content": "overwritten"})
		assert.False(t, errOK(r))
		got, _ := os.ReadFile(filepath.Join(d, "hello.txt"))
		assert.Equal(t, "overwritten", string(got))
	})
	t.Run("custom-mode", func(t *testing.T) {
		d := t.TempDir()
		r := tc(t, srv(t, d), "write_file", map[string]any{"path": "e.sh", "content": "echo hi", "mode": "0755"})
		assert.False(t, errOK(r))
		fi, err := os.Stat(filepath.Join(d, "e.sh"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0755), fi.Mode().Perm())
	})
	t.Run("invalid-mode", func(t *testing.T) {
		d := t.TempDir()
		r := tc(t, srv(t, d), "write_file", map[string]any{"path": "x.txt", "content": "x", "mode": "bad"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "invalid mode")
	})
	t.Run("traversal", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "write_file", map[string]any{"path": "../../../etc/passwd", "content": "evil"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "invalid path")
	})
	t.Run("empty-path", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "write_file", map[string]any{"path": "", "content": "x"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "path is required")
	})
	t.Run("parent-dirs", func(t *testing.T) {
		d := t.TempDir()
		r := tc(t, srv(t, d), "write_file", map[string]any{"path": "a/b/c.txt", "content": "deep"})
		assert.False(t, errOK(r))
		got, _ := os.ReadFile(filepath.Join(d, "a", "b", "c.txt"))
		assert.Equal(t, "deep", string(got))
	})
}

// =============================================================================
// edit_file
// =============================================================================

func TestEditFile(t *testing.T) {
	t.Run("replace", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "edit_file", map[string]any{"path": "hello.txt", "old_text": "hello", "new_text": "hi"})
		assert.Contains(t, rtext(r), `"replaced":true`)
		assert.Contains(t, rtext(r), `"occurrences":1`)
		got, _ := os.ReadFile(filepath.Join(d, "hello.txt"))
		assert.Equal(t, "hi world", string(got))
	})
	t.Run("multiple", func(t *testing.T) {
		d := t.TempDir()
		_ = os.WriteFile(filepath.Join(d, "r.txt"), []byte("foo bar foo baz foo"), 0644)
		r := tc(t, srv(t, d), "edit_file", map[string]any{"path": "r.txt", "old_text": "foo", "new_text": "qux"})
		assert.Contains(t, rtext(r), `"replaced":true`)
		assert.Contains(t, rtext(r), `"occurrences":3`)
		got, _ := os.ReadFile(filepath.Join(d, "r.txt"))
		assert.Equal(t, "qux bar qux baz qux", string(got))
	})
	t.Run("not-found", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "edit_file", map[string]any{"path": "hello.txt", "old_text": "nope", "new_text": "x"})
		assert.Contains(t, rtext(r), `"replaced":false`)
		assert.Contains(t, rtext(r), `"occurrences":0`)
		got, _ := os.ReadFile(filepath.Join(d, "hello.txt"))
		assert.Equal(t, "hello world", string(got))
	})
	t.Run("traversal", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "edit_file", map[string]any{"path": "../../../etc/passwd", "old_text": "x", "new_text": "y"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "invalid path")
	})
	t.Run("empty-path", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "edit_file", map[string]any{"path": "", "old_text": "x", "new_text": "y"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "path is required")
	})
	t.Run("empty-old_text", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "edit_file", map[string]any{"path": "hello.txt", "old_text": "", "new_text": "y"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "old_text is required")
	})
}

// =============================================================================
// delete_file
// =============================================================================

func TestDeleteFile(t *testing.T) {
	t.Run("existing", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "delete_file", map[string]any{"path": "hello.txt"})
		assert.Contains(t, rtext(r), `"deleted":true`)
		_, err := os.Stat(filepath.Join(d, "hello.txt"))
		assert.True(t, os.IsNotExist(err))
	})
	t.Run("not-found", func(t *testing.T) {
		d := t.TempDir()
		r := tc(t, srv(t, d), "delete_file", map[string]any{"path": "nope.txt"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "delete:")
	})
	t.Run("traversal", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "delete_file", map[string]any{"path": "../../../etc/passwd"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "invalid path")
	})
	t.Run("empty-path", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "delete_file", map[string]any{"path": ""})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "path is required")
	})
}

// =============================================================================
// list_dir
// =============================================================================

func TestListDir(t *testing.T) {
	getNames := func(text string) []string {
		var entries []fileEntry
		_ = json.Unmarshal([]byte(text), &entries)
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name
		}
		return names
	}

	t.Run("single-level", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "list_dir", map[string]any{"path": "."})
		names := getNames(rtext(r))
		assert.Contains(t, names, "hello.txt")
		assert.Contains(t, names, "sub")
		assert.NotContains(t, names, "sub/nested.txt")
		assert.NotContains(t, names, ".hidden")
	})
	t.Run("recursive", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "list_dir", map[string]any{"path": ".", "max_depth": float64(5)})
		names := getNames(rtext(r))
		assert.Contains(t, names, "sub/nested.txt")
		assert.Contains(t, names, "hello.txt")
	})
	t.Run("max-depth-capped", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "list_dir", map[string]any{"path": ".", "max_depth": float64(20)})
		assert.False(t, errOK(r))
	})
	t.Run("show-hidden", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "list_dir", map[string]any{"path": ".", "show_hidden": true, "max_depth": float64(5)})
		names := getNames(rtext(r))
		assert.Contains(t, names, ".hidden")
		assert.Contains(t, names, ".hidden_dir")
	})
	t.Run("empty-dir", func(t *testing.T) {
		d := t.TempDir()
		r := tc(t, srv(t, d), "list_dir", map[string]any{"path": "."})
		assert.Equal(t, "[]", strings.TrimSpace(rtext(r)))
	})
	t.Run("traversal", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "list_dir", map[string]any{"path": "../../../etc"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "invalid path")
	})
	t.Run("empty-path", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "list_dir", map[string]any{"path": ""})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "path is required")
	})
}

// =============================================================================
// move_file
// =============================================================================

func TestMoveFile(t *testing.T) {
	t.Run("rename", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "move_file", map[string]any{"from": "hello.txt", "to": "renamed.txt"})
		assert.Contains(t, rtext(r), `"moved":true`)
		_, err := os.Stat(filepath.Join(d, "hello.txt"))
		assert.True(t, os.IsNotExist(err))
		got, _ := os.ReadFile(filepath.Join(d, "renamed.txt"))
		assert.Equal(t, "hello world", string(got))
	})
	t.Run("cross-dir", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "move_file", map[string]any{"from": "hello.txt", "to": "sub/moved.txt"})
		assert.False(t, errOK(r))
		got, _ := os.ReadFile(filepath.Join(d, "sub", "moved.txt"))
		assert.Equal(t, "hello world", string(got))
	})
	t.Run("move-dir", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "move_file", map[string]any{"from": "sub", "to": "renamed_sub"})
		assert.False(t, errOK(r))
		got, _ := os.ReadFile(filepath.Join(d, "renamed_sub", "nested.txt"))
		assert.Equal(t, "nested content", string(got))
	})
	t.Run("src-not-found", func(t *testing.T) {
		d := t.TempDir()
		r := tc(t, srv(t, d), "move_file", map[string]any{"from": "nope.txt", "to": "d.txt"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "move:")
	})
	t.Run("src-traversal", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "move_file", map[string]any{"from": "../../../etc/passwd", "to": "d.txt"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "invalid source")
	})
	t.Run("dst-traversal", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "move_file", map[string]any{"from": "hello.txt", "to": "../../../etc/passwd"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "invalid dest")
	})
	t.Run("empty-from", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "move_file", map[string]any{"from": "", "to": "d.txt"})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "from is required")
	})
	t.Run("empty-to", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "move_file", map[string]any{"from": "hello.txt", "to": ""})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "to is required")
	})
}

// =============================================================================
// search_files
// =============================================================================

func TestSearchFiles(t *testing.T) {
	t.Run("glob-txt", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "search_files", map[string]any{"pattern": "*.txt", "recursive": true})
		var out struct {
			Matches []string `json:"matches"`
			Count   int      `json:"count"`
		}
		_ = json.Unmarshal([]byte(rtext(r)), &out)
		assert.GreaterOrEqual(t, out.Count, 2)
		assert.Contains(t, out.Matches, "hello.txt")
		assert.Contains(t, out.Matches, "sub/nested.txt")
	})
	t.Run("non-recursive", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "search_files", map[string]any{"pattern": "*.txt", "recursive": false})
		var out struct{ Matches []string `json:"matches"` }
		_ = json.Unmarshal([]byte(rtext(r)), &out)
		assert.Contains(t, out.Matches, "hello.txt")
		for _, m := range out.Matches {
			assert.NotContains(t, m, "nested.txt")
		}
	})
	t.Run("subdir-root", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "search_files", map[string]any{"pattern": "*.txt", "root": "sub"})
		var out struct{ Matches []string `json:"matches"` }
		_ = json.Unmarshal([]byte(rtext(r)), &out)
		assert.Contains(t, out.Matches, "nested.txt")
		for _, m := range out.Matches {
			assert.NotEqual(t, "hello.txt", m)
		}
	})
	t.Run("no-match", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "search_files", map[string]any{"pattern": "*.nope"})
		var out struct{ Matches []string `json:"matches"`; Count int `json:"count"` }
		_ = json.Unmarshal([]byte(rtext(r)), &out)
		assert.Equal(t, 0, out.Count)
		assert.Empty(t, out.Matches)
	})
	t.Run("empty-pattern", func(t *testing.T) {
		r := tc(t, srv(t, tmp(t)), "search_files", map[string]any{"pattern": ""})
		assert.True(t, errOK(r))
		assert.Contains(t, rtext(r), "pattern is required")
	})
	t.Run("hidden-file", func(t *testing.T) {
		d := tmp(t)
		r := tc(t, srv(t, d), "search_files", map[string]any{"pattern": ".hidden*"})
		var out struct{ Matches []string `json:"matches"` }
		_ = json.Unmarshal([]byte(rtext(r)), &out)
		assert.Contains(t, out.Matches, ".hidden")
	})
}

// =============================================================================
// js helper
// =============================================================================

func TestJs(t *testing.T) {
	assert.Equal(t, `"hello"`, js("hello"))
	assert.Contains(t, js(map[string]any{"a": 1}), `"a":1`)
	assert.Contains(t, js(fileEntry{Name: "f.txt", Type: "file", Size: 42, Mtime: "2024-01-01T00:00:00Z"}), `"size":42`)
}
