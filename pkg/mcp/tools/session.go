// Package tools implements MCP tool registrations for the agentfs server.
package tools

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/agent-os/agent-os/pkg/sandbox"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Manager singleton
var (
	mgrSingleton *sandbox.Manager
	mgrOnce      sync.Once
)

func getManager(sessionDir string) *sandbox.Manager {
	mgrOnce.Do(func() {
		mgrSingleton = sandbox.NewManager(sessionDir)
	})
	return mgrSingleton
}

// RegisterSessionTools registers all session-management MCP tools on srv.
func RegisterSessionTools(srv *server.MCPServer, sessionDir string, projectRoot string) {
	mgr := getManager(sessionDir)
	registerCreateSession(srv, mgr, projectRoot)
	registerListSessions(srv, mgr)
	registerGetSession(srv, mgr)
	registerCommitSession(srv, mgr)
	registerDiscardSession(srv, mgr)
	registerSessionDiff(srv, mgr)
}

// --- local helpers ---

func sessStrArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func sessBoolArg(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// --- shared result types ---

type commitResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type diffResult struct {
	SessionID string            `json:"session_id"`
	Changes   []sandbox.DiffEntry `json:"changes"`
}

// --- create_session ---

func registerCreateSession(srv *server.MCPServer, mgr *sandbox.Manager, projectRoot string) {
	tool := mcp.NewTool("create_session",
		mcp.WithDescription("Create a new isolated copy-based session for safe file operations"),
		mcp.WithString("project", mcp.Description("Project root directory (defaults to server project root)")),
		mcp.WithString("name", mcp.Description("Optional human-readable name for the session")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		project, _ := sessStrArg(req.GetArguments(), "project")
		if project == "" {
			project = projectRoot
		}
		sess, err := mgr.StartSession(project)
		if err != nil {
			return mcp.NewToolResultError("start session: " + err.Error()), nil
		}
		return jsonTextResult(sess)
	})
}

// --- list_sessions ---

func registerListSessions(srv *server.MCPServer, mgr *sandbox.Manager) {
	tool := mcp.NewTool("list_sessions",
		mcp.WithDescription("List all known sessions and their current status"),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessions, err := mgr.ListSessions()
		if err != nil {
			return mcp.NewToolResultError("list sessions: " + err.Error()), nil
		}
		if sessions == nil {
			sessions = []*sandbox.Session{}
		}
		return jsonTextResult(sessions)
	})
}

// --- get_session ---

func registerGetSession(srv *server.MCPServer, mgr *sandbox.Manager) {
	tool := mcp.NewTool("get_session",
		mcp.WithDescription("Retrieve full details for a single session by ID"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Session ID")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, ok := sessStrArg(req.GetArguments(), "id")
		if !ok || id == "" {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}
		sess, err := mgr.GetSession(id)
		if err != nil {
			return mcp.NewToolResultError("get session: " + err.Error()), nil
		}
		return jsonTextResult(sess)
	})
}

// --- commit_session ---

func registerCommitSession(srv *server.MCPServer, mgr *sandbox.Manager) {
	tool := mcp.NewTool("commit_session",
		mcp.WithDescription("Commit all changes from a session back to the project. Irreversible."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Session ID to commit")),
		mcp.WithString("message", mcp.Description("Optional commit message")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, ok := sessStrArg(req.GetArguments(), "id")
		if !ok || id == "" {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}
		message, _ := sessStrArg(req.GetArguments(), "message")
		if err := mgr.CommitSession(id); err != nil {
			return mcp.NewToolResultError("commit session: " + err.Error()), nil
		}
		return jsonTextResult(commitResult{
			ID:      id,
			Status:  string(sandbox.StatusCommitted),
			Message: message,
		})
	})
}

// --- discard_session ---

func registerDiscardSession(srv *server.MCPServer, mgr *sandbox.Manager) {
	tool := mcp.NewTool("discard_session",
		mcp.WithDescription("Discard a session and all its changes. Requires confirm=true."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Session ID to discard")),
		mcp.WithBoolean("confirm", mcp.Required(),
			mcp.Description("Must be true to proceed; safety guard against accidental discard")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, ok := sessStrArg(req.GetArguments(), "id")
		if !ok || id == "" {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}
		if !sessBoolArg(req.GetArguments(), "confirm") {
			return mcp.NewToolResultError("discard requires confirm=true"), nil
		}
		if err := mgr.DiscardSession(id); err != nil {
			return mcp.NewToolResultError("discard session: " + err.Error()), nil
		}
		return jsonTextResult(commitResult{
			ID:     id,
			Status: string(sandbox.StatusDiscarded),
		})
	})
}

// --- session_diff ---

func registerSessionDiff(srv *server.MCPServer, mgr *sandbox.Manager) {
	tool := mcp.NewTool("session_diff",
		mcp.WithDescription("Show changed files in a session: added, modified, and deleted relative to the original project"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Session ID to diff")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, ok := sessStrArg(req.GetArguments(), "id")
		if !ok || id == "" {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}
		changes, err := mgr.DiffSession(id)
		if err != nil {
			return mcp.NewToolResultError("diff: " + err.Error()), nil
		}
		return jsonTextResult(diffResult{SessionID: id, Changes: changes})
	})
}

// --- json helper ---

// jsonTextResult marshals v to JSON and returns a text-content result.
func jsonTextResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError("marshal: " + err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}
