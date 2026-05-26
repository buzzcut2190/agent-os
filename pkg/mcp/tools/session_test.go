package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sharedSessionDir persists across all tests in the package because
// RegisterSessionTools uses a sync.Once singleton manager whose BaseDir
// is set by the first call.  Using t.TempDir() per test would cause
// subsequent tests to fail after the first test's directory is cleaned up.
var sharedSessionDir string

func TestMain(m *testing.M) {
	var err error
	sharedSessionDir, err = os.MkdirTemp("", "session-tools-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create shared session dir: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(sharedSessionDir)
	os.Exit(code)
}

// requireOverlayMount skips the test if overlayfs is unavailable or the
// current user cannot mount.
func requireOverlayMount(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/sys/module/overlay"); err != nil {
		t.Skip("overlay kernel module not available")
	}
	lower := t.TempDir()
	upper := t.TempDir()
	work := t.TempDir()
	merged := t.TempDir()
	for _, dir := range []string{upper, work, merged} {
		_ = os.MkdirAll(dir, 0755)
	}
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lower, upper, work)
	if err := syscall.Mount("overlay", merged, "overlay", 0, opts); err != nil {
		t.Skip("overlay mount not permitted: " + err.Error())
	}
	_ = syscall.Unmount(merged, 0)
}

// setupSessionServer creates a fresh MCPServer with session tools registered
// and a temporary project directory containing a test file.
func setupSessionServer(t *testing.T) (*server.MCPServer, string) {
	t.Helper()
	projectDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(projectDir, "test.txt"), []byte("test"), 0644)
	srv := server.NewMCPServer("test", "0.0.1")
	RegisterSessionTools(srv, sharedSessionDir, projectDir)
	return srv, projectDir
}

// callSessionTool invokes a registered tool by name and returns the result.
func callSessionTool(t *testing.T, srv *server.MCPServer, name string, args map[string]any) *mcp.CallToolResult {
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

// getResultText extracts the text content from a CallToolResult.
func getResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content, "expected non-empty content")
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent in result")
	return tc.Text
}

// skipIfCreateFailed gracefully skips the test when create_session fails
// (e.g. because the overlay mount is not permitted at runtime).
func skipIfCreateFailed(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if !result.IsError {
		return
	}
	text := getResultText(t, result)
	if strings.Contains(text, "mount") || strings.Contains(text, "overlay") {
		t.Skip("create_session failed (overlay issue): " + text)
	}
	t.Fatalf("create_session returned unexpected error: %s", text)
}

// =============================================================================
// Tests
// =============================================================================

func TestCreateSession(t *testing.T) {
	requireOverlayMount(t)
	srv, projectDir := setupSessionServer(t)

	result := callSessionTool(t, srv, "create_session", map[string]any{"project": projectDir})
	skipIfCreateFailed(t, result)

	text := getResultText(t, result)
	var session map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &session))
	assert.NotEmpty(t, session["id"], "session id should not be empty")
	assert.NotEmpty(t, session["project"], "session project should not be empty")
	assert.Equal(t, "active", session["status"], "session status should be active")
}

func TestListSessions(t *testing.T) {
	requireOverlayMount(t)
	srv, projectDir := setupSessionServer(t)

	r1 := callSessionTool(t, srv, "create_session", map[string]any{"project": projectDir})
	skipIfCreateFailed(t, r1)
	r2 := callSessionTool(t, srv, "create_session", map[string]any{"project": projectDir})
	skipIfCreateFailed(t, r2)

	result := callSessionTool(t, srv, "list_sessions", nil)
	require.False(t, result.IsError, "list_sessions returned error: %s", getResultText(t, result))

	text := getResultText(t, result)
	var sessions []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &sessions))
	assert.GreaterOrEqual(t, len(sessions), 2, "expected at least 2 sessions; got %d", len(sessions))
}

func TestGetSession(t *testing.T) {
	requireOverlayMount(t)
	srv, projectDir := setupSessionServer(t)

	r := callSessionTool(t, srv, "create_session", map[string]any{"project": projectDir})
	skipIfCreateFailed(t, r)

	var session map[string]any
	require.NoError(t, json.Unmarshal([]byte(getResultText(t, r)), &session))
	sessionID, _ := session["id"].(string)

	// Get existing session — should succeed and return full details.
	result := callSessionTool(t, srv, "get_session", map[string]any{"id": sessionID})
	require.False(t, result.IsError, "get_session returned error: %s", getResultText(t, result))

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(getResultText(t, result)), &got))
	assert.Equal(t, sessionID, got["id"])
	assert.Equal(t, session["project"], got["project"])

	// Get non-existent session — should return an error result.
	result = callSessionTool(t, srv, "get_session", map[string]any{"id": "nonexistent-session-id"})
	assert.True(t, result.IsError, "expected error for non-existent session ID")
}

func TestDiscardSession(t *testing.T) {
	requireOverlayMount(t)
	srv, projectDir := setupSessionServer(t)

	r := callSessionTool(t, srv, "create_session", map[string]any{"project": projectDir})
	skipIfCreateFailed(t, r)

	var session map[string]any
	require.NoError(t, json.Unmarshal([]byte(getResultText(t, r)), &session))
	sessionID, _ := session["id"].(string)

	// confirm=false must be rejected as a safety guard.
	result := callSessionTool(t, srv, "discard_session", map[string]any{
		"id":      sessionID,
		"confirm": false,
	})
	assert.True(t, result.IsError, "confirm=false should be rejected")
	assert.Contains(t, getResultText(t, result), "confirm=true",
		"error message should mention confirm=true")

	// confirm=true must succeed and set status to discarded.
	result = callSessionTool(t, srv, "discard_session", map[string]any{
		"id":      sessionID,
		"confirm": true,
	})
	require.False(t, result.IsError, "discard_session error: %s", getResultText(t, result))

	var dr map[string]any
	require.NoError(t, json.Unmarshal([]byte(getResultText(t, result)), &dr))
	assert.Equal(t, sessionID, dr["id"])
	assert.Equal(t, "discarded", dr["status"])
}

func TestSessionDiff(t *testing.T) {
	requireOverlayMount(t)
	srv, projectDir := setupSessionServer(t)

	r := callSessionTool(t, srv, "create_session", map[string]any{"project": projectDir})
	skipIfCreateFailed(t, r)

	var session map[string]any
	require.NoError(t, json.Unmarshal([]byte(getResultText(t, r)), &session))
	sessionID, _ := session["id"].(string)
	mergedPath, _ := session["merged"].(string)

	// Write a new file through the overlay merged path.
	newFilePath := filepath.Join(mergedPath, "new_file.txt")
	require.NoError(t, os.WriteFile(newFilePath, []byte("new content"), 0644))

	// Get the diff — the new file should appear as "added".
	result := callSessionTool(t, srv, "session_diff", map[string]any{"id": sessionID})
	require.False(t, result.IsError, "session_diff error: %s", getResultText(t, result))

	var diff map[string]any
	require.NoError(t, json.Unmarshal([]byte(getResultText(t, result)), &diff))
	assert.Equal(t, sessionID, diff["session_id"])

	changes, ok := diff["changes"].([]any)
	require.True(t, ok, "changes should be an array")
	require.GreaterOrEqual(t, len(changes), 1, "expected at least 1 change; got %d", len(changes))

	// Verify new_file.txt is listed as "added".
	found := false
	for _, c := range changes {
		change, _ := c.(map[string]any)
		if change["path"] == "new_file.txt" && change["status"] == "added" {
			found = true
			break
		}
	}
	assert.True(t, found, "new_file.txt should appear as 'added' in session diff")
}
