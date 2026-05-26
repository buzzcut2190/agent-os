package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/agent-os/agent-os/pkg/context"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// excludedDirs lists directory names automatically skipped during file walks.
var excludedDirs = []string{".git", "node_modules", "vendor", "__pycache__", ".cache", "dist", "build", "target"}

// isExcluded returns true when any path segment matches an excluded directory.
func isExcluded(relPath string) bool {
	for _, part := range strings.Split(relPath, string(filepath.Separator)) {
		for _, ex := range excludedDirs {
			if part == ex {
				return true
			}
		}
	}
	return false
}

// extToLang maps common file extensions to language names.
var extToLang = map[string]string{
	".go": "Go", ".py": "Python", ".ts": "TypeScript", ".tsx": "TypeScript",
	".js": "JavaScript", ".jsx": "JavaScript", ".rs": "Rust", ".java": "Java",
	".c": "C", ".h": "C", ".cpp": "C++", ".hpp": "C++", ".cc": "C++",
	".rb": "Ruby", ".sh": "Shell", ".bash": "Shell", ".zsh": "Shell",
}

// fileDesc holds metadata about a single file returned by get_file_summary.
type fileDesc struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Lines int    `json:"lines"`
	Mtime string `json:"mtime"`
}

// locResult is the structured output for count_loc.
type locResult struct {
	TotalLines int                        `json:"total_lines"`
	TotalFiles int                        `json:"total_files"`
	ByLanguage map[string]locLanguageStat `json:"by_language"`
}

type locLanguageStat struct {
	Files int `json:"files"`
	Lines int `json:"lines"`
}

// countFileLines returns the number of lines in the file at path.
func countFileLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		n++
	}
	return n, sc.Err()
}

// RegisterContextTools registers all context-related MCP tools on srv.
func RegisterContextTools(srv *server.MCPServer, projectRoot string) {
	registerGetContext(srv, projectRoot)
	registerGetFileSummary(srv, projectRoot)
	registerDetectStack(srv, projectRoot)
	registerCountLoc(srv, projectRoot)
}

// ---------------------------------------------------------------------------
// get_context
// ---------------------------------------------------------------------------

func registerGetContext(srv *server.MCPServer, root string) {
	tool := mcp.NewTool("get_context",
		mcp.WithDescription(
			"Generate a comprehensive project context summary: detected languages, "+
				"directory tree, key files, dependencies, and file statistics. "+
				"Set force_refresh=true to bypass the internal cache and force a fresh scan.",
		),
		mcp.WithBoolean("force_refresh",
			mcp.Description("When true, force regeneration of the summary."),
		),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		engine := pkgctx.NewEngine(root)
		if req.GetBool("force_refresh", false) {
			engine.Invalidate()
		}
		summary, err := engine.GetSummary()
		if err != nil {
			return mcp.NewToolResultError("failed to generate project summary: " + err.Error()), nil
		}
		data, err := json.Marshal(summary)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("serialization error: %v", err)), nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(data)}},
		}, nil
	})
}

// ---------------------------------------------------------------------------
// get_file_summary
// ---------------------------------------------------------------------------

func registerGetFileSummary(srv *server.MCPServer, root string) {
	tool := mcp.NewTool("get_file_summary",
		mcp.WithDescription(
			"Return file metadata: absolute path, size in bytes, line count, and last-modified timestamp.",
		),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Relative or absolute path within the project root."),
		),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		abs, err := safePath(root, path)
		if err != nil {
			return mcp.NewToolResultError("invalid path: " + err.Error()), nil
		}
		info, err := os.Stat(abs)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot stat %q: %v", path, err)), nil
		}
		lines := 0
		if !info.IsDir() {
			lines, _ = countFileLines(abs)
		}
		fd := fileDesc{
			Path:  abs,
			Size:  info.Size(),
			Lines: lines,
			Mtime: info.ModTime().Format(time.RFC3339),
		}
		data, _ := json.Marshal(fd)
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(data)}},
		}, nil
	})
}

// ---------------------------------------------------------------------------
// detect_stack
// ---------------------------------------------------------------------------

func registerDetectStack(srv *server.MCPServer, root string) {
	tool := mcp.NewTool("detect_stack",
		mcp.WithDescription(
			"Detect the technology stack used by the project. Returns a list of languages "+
				"with their build systems and entry files.",
		),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		types := pkgctx.DetectLanguages(root)
		data, err := json.Marshal(types)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("serialization error: %v", err)), nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(data)}},
		}, nil
	})
}

// ---------------------------------------------------------------------------
// count_loc
// ---------------------------------------------------------------------------

func registerCountLoc(srv *server.MCPServer, root string) {
	tool := mcp.NewTool("count_loc",
		mcp.WithDescription(
			"Count lines of code in the project, optionally scoped to a subdirectory "+
				"or filtered by language. Returns total lines, file count, and per-language breakdown.",
		),
		mcp.WithString("path",
			mcp.Description("Relative subdirectory to scope the count (defaults to project root)."),
		),
		mcp.WithString("language",
			mcp.Description("Filter to a specific language, e.g. 'Go', 'Python', 'TypeScript'."),
		),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		scanRoot := root
		if pathArg := req.GetString("path", ""); pathArg != "" {
			abs, err := safePath(root, pathArg)
			if err != nil {
				return mcp.NewToolResultError("invalid path: " + err.Error()), nil
			}
			scanRoot = abs
		}
		filterLang := strings.ToLower(req.GetString("language", ""))

		result := locResult{ByLanguage: make(map[string]locLanguageStat)}
		err := filepath.WalkDir(scanRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, relErr := filepath.Rel(scanRoot, path)
			if relErr != nil || rel == "." {
				return nil
			}
			if d.IsDir() && isExcluded(rel) {
				return fs.SkipDir
			}
			if d.IsDir() {
				return nil
			}

			lang := extToLang[strings.ToLower(filepath.Ext(path))]
			if lang == "" {
				lang = "Other"
			}
			if filterLang != "" && !strings.EqualFold(lang, filterLang) {
				return nil
			}
			lines, err := countFileLines(path)
			if err != nil {
				return nil
			}
			result.TotalFiles++
			result.TotalLines += lines
			s := result.ByLanguage[lang]
			s.Files++
			s.Lines += lines
			result.ByLanguage[lang] = s
			return nil
		})
		if err != nil {
			return mcp.NewToolResultError("walk error: " + err.Error()), nil
		}
		data, _ := json.Marshal(result)
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(data)}},
		}, nil
	})
}
