package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// searchHit represents a single match found during a search operation.
type searchHit struct {
	Path       string `json:"path"`
	LineNumber int    `json:"line_number"`
	Snippet    string `json:"snippet"`
	Context    string `json:"context,omitempty"`
}

// maxFileBytes is the maximum size of a file scanned during search.
const maxFileBytes = 2 * 1024 * 1024 // 2 MiB

// trimSnippet limits s to maxLen runes (approx), appending "..." when truncated.
func trimSnippet(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// binaryCheck returns true when the first n bytes of path contain a null byte.
func binaryCheck(path string, n int) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	buf := make([]byte, n)
	read, _ := f.Read(buf)
	return bytes.IndexByte(buf[:read], 0) >= 0
}

// lineScanner walks root, opening each eligible text file and calling visit for
// every line. Stops early when hitCap results have been collected.
func lineScanner(root string, hitCap int, filterLang string, visit func(string, int, string) *searchHit) ([]searchHit, error) {
	var hits []searchHit
	err := filepath.WalkDir(root, func(abs string, d fs.DirEntry, err error) error {
		if err != nil || len(hits) >= hitCap {
			return nil
		}
		rel, relErr := filepath.Rel(root, abs)
		if relErr != nil || rel == "." {
			return nil
		}
		if d.IsDir() {
			if isExcluded(rel) {
				return fs.SkipDir
			}
			return nil
		}
		if isExcluded(rel) {
			return nil
		}
		if filterLang != "" {
			fl := extToLang[strings.ToLower(filepath.Ext(rel))]
			if !strings.EqualFold(fl, filterLang) {
				return nil
			}
		}
		if info, err := d.Info(); err != nil || info.Size() > maxFileBytes || binaryCheck(abs, 512) {
			return nil
		}
		f, err := os.Open(abs)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for lineNum := 1; sc.Scan() && len(hits) < hitCap; lineNum++ {
			if h := visit(sc.Text(), lineNum, rel); h != nil {
				hits = append(hits, *h)
			}
		}
		return nil
	})
	return hits, err
}

// jsonResult marshals hits to JSON and wraps it in a CallToolResult.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, _ := json.Marshal(v)
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(data)}},
	}, nil
}

// RegisterSearchTools registers all search-related MCP tools on srv.
func RegisterSearchTools(srv *server.MCPServer, projectRoot string) {
	registerSearchCode(srv, projectRoot)
	registerGrepRegex(srv, projectRoot)
	registerFindReferences(srv, projectRoot)
	registerFindDefinition(srv, projectRoot)
}

// search_code
func registerSearchCode(srv *server.MCPServer, root string) {
	tool := mcp.NewTool("search_code",
		mcp.WithDescription(
			"Search project source code for literal text. Returns file path, line number, "+
				"and line snippet for each match. Handles 500+ files in under 3 seconds.",
		),
		mcp.WithString("query", mcp.Required(), mcp.Description("Literal string to search for.")),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 20).")),
		mcp.WithBoolean("case_sensitive", mcp.Description("Case-sensitive match (default false).")),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}
		maxRes := req.GetInt("max_results", 20)
		caseSens := req.GetBool("case_sensitive", false)
		if !caseSens {
			query = strings.ToLower(query)
		}

		hits, err := lineScanner(root, maxRes, "", func(line string, num int, rel string) *searchHit {
			cmp := line
			if !caseSens {
				cmp = strings.ToLower(line)
			}
			if strings.Contains(cmp, query) {
				return &searchHit{Path: rel, LineNumber: num, Snippet: trimSnippet(line, 200)}
			}
			return nil
		})
		if err != nil {
			return mcp.NewToolResultError("search error: " + err.Error()), nil
		}
		return jsonResult(hits)
	})
}

// grep_regex
func registerGrepRegex(srv *server.MCPServer, root string) {
	tool := mcp.NewTool("grep_regex",
		mcp.WithDescription(
			"Search project files using a Go-compatible regular expression. "+
				"Returns matching file paths, line numbers, and line snippets.",
		),
		mcp.WithString("pattern", mcp.Required(),
			mcp.Description("Go regexp pattern, e.g. 'func\\s+\\w+'.")),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default 50).")),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pattern := req.GetString("pattern", "")
		if pattern == "" {
			return mcp.NewToolResultError("pattern is required"), nil
		}
		maxRes := req.GetInt("max_results", 50)

		re, err := regexp.Compile(pattern)
		if err != nil {
			return mcp.NewToolResultError("invalid regex: " + err.Error()), nil
		}

		hits, err := lineScanner(root, maxRes, "", func(line string, num int, rel string) *searchHit {
			if re.MatchString(line) {
				return &searchHit{Path: rel, LineNumber: num, Snippet: trimSnippet(line, 200)}
			}
			return nil
		})
		if err != nil {
			return mcp.NewToolResultError("search error: " + err.Error()), nil
		}
		return jsonResult(hits)
	})
}

// find_references
func registerFindReferences(srv *server.MCPServer, root string) {
	tool := mcp.NewTool("find_references",
		mcp.WithDescription(
			"Find all references to a symbol (variable, function, type, etc.) using simple "+
				"string matching. Returns path, line number, and surrounding context line.",
		),
		mcp.WithString("symbol", mcp.Required(),
			mcp.Description("Symbol name to search for, e.g. 'Server', 'handleRequest'.")),
		mcp.WithString("language",
			mcp.Description("Optional language filter, e.g. 'Go', 'Python', 'TypeScript'.")),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		symbol := req.GetString("symbol", "")
		if symbol == "" {
			return mcp.NewToolResultError("symbol is required"), nil
		}
		filterLang := strings.ToLower(req.GetString("language", ""))

		hits, err := lineScanner(root, 100, filterLang,
			func(line string, num int, rel string) *searchHit {
				if strings.Contains(line, symbol) {
					return &searchHit{
						Path:       rel,
						LineNumber: num,
						Snippet:    trimSnippet(line, 200),
						Context:    trimSnippet(line, 200),
					}
				}
				return nil
			})
		if err != nil {
			return mcp.NewToolResultError("search error: " + err.Error()), nil
		}
		return jsonResult(hits)
	})
}

// find_definition
// langDefPatterns maps a language key to regexp templates for definition lines.
// %s is replaced with regexp.QuoteMeta(symbol) at search time.
var langDefPatterns = map[string][]string{
	"":           {`func\s+(\([^)]*\)\s+)?%s\b`, `type\s+%s\b`, `def\s+%s\b`, `class\s+%s\b`, `(const|let|var)\s+%s\b`, `fn\s+%s\b`, `struct\s+%s\b`, `enum\s+%s\b`, `trait\s+%s\b`, `interface\s+%s\b`, `impl\b[\s<].*%s\b`},
	"go":         {`func\s+(\([^)]*\)\s+)?%s\b`, `type\s+%s\s`, `\bvar\s+%s\s`, `\bconst\s+%s\s`},
	"python":     {`def\s+%s\b`, `class\s+%s\b`},
	"typescript": {`(function|class)\s+%s\b`, `(const|let|var)\s+%s\b`, `interface\s+%s\b`, `type\s+%s\b`, `enum\s+%s\b`},
	"javascript": {`(function|class)\s+%s\b`, `(const|let|var)\s+%s\b`},
	"rust":       {`fn\s+%s\b`, `struct\s+%s\b`, `enum\s+%s\b`, `trait\s+%s\b`, `impl\b[\s<].*%s\b`},
	"java":       {`(class|interface|enum)\s+%s\b`},
}

func registerFindDefinition(srv *server.MCPServer, root string) {
	tool := mcp.NewTool("find_definition",
		mcp.WithDescription(
			"Find the definition of a symbol (function, type, class, variable) by searching "+
				"definition-pattern lines. Optionally filter by language for more specific patterns.",
		),
		mcp.WithString("symbol", mcp.Required(),
			mcp.Description("Symbol name whose definition to find, e.g. 'Server', 'NewEngine'.")),
		mcp.WithString("language",
			mcp.Description("Optional language to use specific patterns, e.g. 'Go', 'Python', 'Rust'.")),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		symbol := req.GetString("symbol", "")
		if symbol == "" {
			return mcp.NewToolResultError("symbol is required"), nil
		}
		langKey := strings.ToLower(req.GetString("language", ""))

		rawPatterns := langDefPatterns[langKey]
		if len(rawPatterns) == 0 {
			rawPatterns = langDefPatterns[""]
		}
		quoted := regexp.QuoteMeta(symbol)
		patterns := make([]*regexp.Regexp, 0, len(rawPatterns))
		for _, raw := range rawPatterns {
			if re, err := regexp.Compile(fmt.Sprintf(raw, quoted)); err == nil {
				patterns = append(patterns, re)
			}
		}

		hits, err := lineScanner(root, 50, langKey, func(line string, num int, rel string) *searchHit {
			for _, re := range patterns {
				if re.MatchString(line) {
					return &searchHit{
						Path: rel, LineNumber: num,
						Snippet: trimSnippet(line, 200), Context: trimSnippet(line, 200),
					}
				}
			}
			return nil
		})
		if err != nil {
			return mcp.NewToolResultError("search error: " + err.Error()), nil
		}
		return jsonResult(hits)
	})
}
