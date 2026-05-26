package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func safePath(root, input string) (string, error) {
	if input == "" {
		return "", errors.New("empty path")
	}
	abs := input
	if !filepath.IsAbs(input) {
		abs = filepath.Join(root, input)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("path escapes project root")
	}
	for _, p := range strings.Split(rel, string(filepath.Separator)) {
		if p == ".." {
			return "", errors.New("path escapes project root")
		}
	}
	return abs, nil
}
type fileEntry struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"`
}
func js(v any) string { b, _ := json.Marshal(v); return string(b) }

// RegisterFileOps registers all file operation MCP tools on the server.
func RegisterFileOps(srv *server.MCPServer, projectRoot string) {
	// read_file
	srv.AddTool(mcp.NewTool("read_file",
		mcp.WithDescription("Read file contents with optional offset and limit."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path relative to project root")),
		mcp.WithNumber("offset", mcp.Description("Byte offset (default: 0)")),
		mcp.WithNumber("limit", mcp.Description("Max bytes (default: to EOF)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		p, _ := args["path"].(string)
		if p == "" { return mcp.NewToolResultError("path is required"), nil }
		s, err := safePath(projectRoot, p)
		if err != nil { return mcp.NewToolResultError("invalid path: "+err.Error()), nil }
		data, err := os.ReadFile(s)
		if err != nil { return mcp.NewToolResultError("read: "+err.Error()), nil }
		off := 0
		if f, _ := args["offset"].(float64); f > 0 { off = int(f) }
		if off > len(data) { off = len(data) }
		lim := len(data) - off
		if f, _ := args["limit"].(float64); f > 0 { lim = int(f) }
		end := off + lim
		if end > len(data) { end = len(data) }
		return mcp.NewToolResultText(string(data[off:end])), nil
	})
	// write_file
	srv.AddTool(mcp.NewTool("write_file",
		mcp.WithDescription("Write content to a file, creating parent directories."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path relative to project root")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Content to write")),
		mcp.WithString("mode", mcp.Description("Octal permission mode (default: 0644)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		p, _ := args["path"].(string)
		if p == "" { return mcp.NewToolResultError("path is required"), nil }
		c, _ := args["content"].(string)
		s, err := safePath(projectRoot, p)
		if err != nil { return mcp.NewToolResultError("invalid path: "+err.Error()), nil }
		ms := "0644"
		if v, _ := args["mode"].(string); v != "" { ms = v }
		mode, err := strconv.ParseUint(ms, 8, 32)
		if err != nil { return mcp.NewToolResultError("invalid mode: "+err.Error()), nil }
		if err := os.MkdirAll(filepath.Dir(s), 0755); err != nil {
			return mcp.NewToolResultError("mkdir: "+err.Error()), nil
		}
		if err := os.WriteFile(s, []byte(c), os.FileMode(mode)); err != nil {
			return mcp.NewToolResultError("write: "+err.Error()), nil
		}
		return mcp.NewToolResultText(js(map[string]any{
			"written": true, "path": s, "size": len(c),
		})), nil
	})
	// edit_file
	srv.AddTool(mcp.NewTool("edit_file",
		mcp.WithDescription("Find and replace text in a file."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path relative to project root")),
		mcp.WithString("old_text", mcp.Required(), mcp.Description("Exact text to find")),
		mcp.WithString("new_text", mcp.Required(), mcp.Description("Text to replace with")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		p, _ := args["path"].(string)
		if p == "" { return mcp.NewToolResultError("path is required"), nil }
		old, _ := args["old_text"].(string)
		if old == "" { return mcp.NewToolResultError("old_text is required"), nil }
		nw, _ := args["new_text"].(string)
		s, err := safePath(projectRoot, p)
		if err != nil { return mcp.NewToolResultError("invalid path: "+err.Error()), nil }
		data, err := os.ReadFile(s)
		if err != nil { return mcp.NewToolResultError("read: "+err.Error()), nil }
		orig := string(data)
		n := strings.Count(orig, old)
		if n == 0 {
			return mcp.NewToolResultText(js(map[string]any{
				"replaced": false, "occurrences": 0, "message": "old_text not found",
			})), nil
		}
		info, _ := os.Stat(s)
		if err := os.WriteFile(s, []byte(strings.ReplaceAll(orig, old, nw)), info.Mode()); err != nil {
			return mcp.NewToolResultError("write: "+err.Error()), nil
		}
		return mcp.NewToolResultText(js(map[string]any{
			"replaced": true, "occurrences": n,
		})), nil
	})
	// delete_file
	srv.AddTool(mcp.NewTool("delete_file",
		mcp.WithDescription("Delete a file."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path relative to project root")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		p, _ := args["path"].(string)
		if p == "" { return mcp.NewToolResultError("path is required"), nil }
		s, err := safePath(projectRoot, p)
		if err != nil { return mcp.NewToolResultError("invalid path: "+err.Error()), nil }
		if err := os.Remove(s); err != nil {
			return mcp.NewToolResultError("delete: "+err.Error()), nil
		}
		return mcp.NewToolResultText(js(map[string]any{"deleted": true, "path": s})), nil
	})
	// list_dir
	srv.AddTool(mcp.NewTool("list_dir",
		mcp.WithDescription("List directory contents recursively (max depth 10)."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Dir path relative to project root")),
		mcp.WithNumber("max_depth", mcp.Description("Recursion depth (default: 1)")),
		mcp.WithBoolean("show_hidden", mcp.Description("Include hidden files (default: false)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		p, _ := args["path"].(string)
		if p == "" { return mcp.NewToolResultError("path is required"), nil }
		s, err := safePath(projectRoot, p)
		if err != nil { return mcp.NewToolResultError("invalid path: "+err.Error()), nil }
		md := 1
		if f, _ := args["max_depth"].(float64); f > 0 { md = int(f) }
		if md > 10 { md = 10 }
		sh, _ := args["show_hidden"].(bool)
		var walk func(dir string, depth int) ([]fileEntry, error)
		walk = func(dir string, depth int) ([]fileEntry, error) {
			entries, err := os.ReadDir(dir)
			if err != nil { return nil, err }
			var out []fileEntry
			for _, e := range entries {
				n := e.Name()
				if !sh && strings.HasPrefix(n, ".") { continue }
				info, ierr := e.Info()
				if ierr != nil { continue }
				typ := "file"
				if e.IsDir() { typ = "dir" } else if e.Type()&os.ModeSymlink != 0 { typ = "symlink" }
				full := filepath.Join(dir, n)
				rel, _ := filepath.Rel(projectRoot, full)
				out = append(out, fileEntry{
					Name: rel, Type: typ, Size: info.Size(),
					Mtime: info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
				})
				if e.IsDir() && depth < md {
					sub, _ := walk(full, depth+1)
					out = append(out, sub...)
				}
			}
			return out, nil
		}
		entries, err := walk(s, 1)
		if err != nil { return mcp.NewToolResultError("list: "+err.Error()), nil }
		if entries == nil { entries = []fileEntry{} }
		return mcp.NewToolResultText(js(entries)), nil
	})
	// move_file
	srv.AddTool(mcp.NewTool("move_file",
		mcp.WithDescription("Move or rename a file or directory."),
		mcp.WithString("from", mcp.Required(), mcp.Description("Source path relative to project root")),
		mcp.WithString("to", mcp.Required(), mcp.Description("Destination path relative to project root")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		fr, _ := args["from"].(string)
		if fr == "" { return mcp.NewToolResultError("from is required"), nil }
		to, _ := args["to"].(string)
		if to == "" { return mcp.NewToolResultError("to is required"), nil }
		sf, err := safePath(projectRoot, fr)
		if err != nil { return mcp.NewToolResultError("invalid source: "+err.Error()), nil }
		st, err := safePath(projectRoot, to)
		if err != nil { return mcp.NewToolResultError("invalid dest: "+err.Error()), nil }
		if err := os.MkdirAll(filepath.Dir(st), 0755); err != nil {
			return mcp.NewToolResultError("mkdir: "+err.Error()), nil
		}
		if err := os.Rename(sf, st); err != nil {
			return mcp.NewToolResultError("move: "+err.Error()), nil
		}
		return mcp.NewToolResultText(js(map[string]any{
			"moved": true, "from": sf, "to": st,
		})), nil
	})
	// search_files
	srv.AddTool(mcp.NewTool("search_files",
		mcp.WithDescription("Search files by glob pattern (e.g. *.go)."),
		mcp.WithString("pattern", mcp.Required(), mcp.Description("Glob pattern to match")),
		mcp.WithString("root", mcp.Description("Search root (default: project root)")),
		mcp.WithBoolean("recursive", mcp.Description("Walk tree (default: true)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		pat, _ := args["pattern"].(string)
		if pat == "" { return mcp.NewToolResultError("pattern is required"), nil }
		sr := projectRoot
		if v, _ := args["root"].(string); v != "" { sr, _ = safePath(projectRoot, v) }
		rec := true
		if b, ok := args["recursive"].(bool); ok { rec = b }
		var matches []string
		if rec {
			_ = filepath.Walk(sr, func(path string, info os.FileInfo, err error) error {
				if err != nil { return nil }
				if matched, _ := filepath.Match(pat, filepath.Base(path)); matched {
					rel, _ := filepath.Rel(sr, path)
					matches = append(matches, rel)
				}
				return nil
			})
		} else {
			found, _ := filepath.Glob(filepath.Join(sr, pat))
			for _, f := range found {
				rel, _ := filepath.Rel(sr, f)
				matches = append(matches, rel)
			}
		}
		if matches == nil { matches = []string{} }
		return mcp.NewToolResultText(js(map[string]any{
			"pattern": pat, "root": sr, "matches": matches, "count": len(matches),
		})), nil
	})
}
