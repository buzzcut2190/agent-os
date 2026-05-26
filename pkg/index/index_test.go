package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func setupGoProject(t *testing.T) string {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "lib"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module test\n\ngo 1.22\n\nrequire github.com/pkg/errors v0.9.1\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(hello())\n}\n\nfunc hello() string {\n\treturn \"hi\"\n}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lib", "lib.go"),
		[]byte("package lib\n\ntype Handler struct{}\n\nfunc (h *Handler) Serve() string {\n\treturn main.hello()\n}\n"), 0o644))
	return dir
}

func symbolMap(symbols []Symbol) map[string]Symbol {
	m := make(map[string]Symbol, len(symbols))
	for _, s := range symbols {
		m[s.Name] = s
	}
	return m
}

// ---------------------------------------------------------------------------
// TestGoAnalyzer
// ---------------------------------------------------------------------------

func TestGoAnalyze(t *testing.T) {
	dir := setupGoProject(t)
	a := NewGoAnalyzer()

	result, err := a.Analyze(dir)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "go", result.Language)
	assert.NotEmpty(t, result.Symbols, "expected non-empty symbols")
	assert.NotEmpty(t, result.Imports, "expected non-empty imports")
	assert.NotEmpty(t, result.CallGraph, "expected non-empty call graph")
	assert.False(t, result.IndexedAt.IsZero(), "expected IndexedAt to be set")
}

func TestGoSymbols(t *testing.T) {
	dir := setupGoProject(t)
	a := NewGoAnalyzer()

	result, err := a.Analyze(dir)
	require.NoError(t, err)

	byName := symbolMap(result.Symbols)

	// --- main.go symbols ---
	sym, ok := byName[".hello"]
	require.True(t, ok, "expected symbol .hello")
	assert.Equal(t, KindFunction, sym.Kind)
	assert.Equal(t, "main", sym.Package)
	assert.Contains(t, sym.Signature, "func()")

	sym, ok = byName[".main"]
	require.True(t, ok, "expected symbol .main")
	assert.Equal(t, KindFunction, sym.Kind)
	assert.Equal(t, "main", sym.Package)

	// --- lib/lib.go symbols ---
	sym, ok = byName["lib.Handler"]
	require.True(t, ok, "expected symbol lib.Handler")
	assert.Equal(t, KindClass, sym.Kind)
	assert.Equal(t, "lib", sym.Package)

	sym, ok = byName["lib.(*Handler).Serve"]
	require.True(t, ok, "expected symbol lib.(*Handler).Serve")
	assert.Equal(t, KindMethod, sym.Kind)
	assert.Equal(t, "lib", sym.Package)
	assert.Contains(t, sym.Signature, "Handler")
}

func TestGoCallGraph(t *testing.T) {
	dir := setupGoProject(t)
	a := NewGoAnalyzer()

	result, err := a.Analyze(dir)
	require.NoError(t, err)

	// Build a set for easy lookup: "caller|callee"
	edges := make(map[string]CallEdge, len(result.CallGraph))
	for _, e := range result.CallGraph {
		edges[e.Caller+"|"+e.Callee] = e
	}

	// main -> hello (bare-ident call)
	e, ok := edges[".main|hello"]
	require.True(t, ok, "expected call edge .main -> hello")
	assert.Equal(t, "main.go", e.File)

	// main -> fmt.Println (selector-expr call)
	_, ok = edges[".main|fmt.Println"]
	assert.True(t, ok, "expected call edge .main -> fmt.Println")

	// Serve -> main.hello (cross-package-selector call)
	_, ok = edges["lib.(*Handler).Serve|main.hello"]
	assert.True(t, ok, "expected call edge lib.(*Handler).Serve -> main.hello")
}

func TestGoImports(t *testing.T) {
	dir := setupGoProject(t)
	a := NewGoAnalyzer()

	result, err := a.Analyze(dir)
	require.NoError(t, err)

	// Collect import targets from root (pkgDir = ".")
	var fmtImport bool
	for _, imp := range result.Imports {
		if imp.From == "." && imp.To == "fmt" && imp.Kind == "import" {
			fmtImport = true
			break
		}
	}
	assert.True(t, fmtImport, "expected import of fmt from root package")

	// Verify Dependencies: fmt is stdlib, so external deps should be empty
	assert.Empty(t, result.Dependencies, "expected no external dependencies (only stdlib imports)")
}

func TestGoSyntaxError(t *testing.T) {
	dir := t.TempDir()

	// A file with a syntax error
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.go"),
		[]byte("package main\n\nfunc {{{}\n"), 0o644))
	// A valid file in the same directory
	require.NoError(t, os.WriteFile(filepath.Join(dir, "good.go"),
		[]byte("package main\n\nfunc ok() {}\n"), 0o644))

	a := NewGoAnalyzer()
	result, err := a.Analyze(dir)
	require.NoError(t, err, "analyzer should not error on syntax errors")
	require.NotNil(t, result)

	byName := symbolMap(result.Symbols)
	_, ok := byName[".ok"]
	assert.True(t, ok, "symbol from valid file should be present despite syntax error in another file")
}

// ---------------------------------------------------------------------------
// TestEngine
// ---------------------------------------------------------------------------

func TestEngineCache(t *testing.T) {
	dir := setupGoProject(t)
	engine := NewEngine(dir)

	r1, err := engine.GetAnalysis()
	require.NoError(t, err)
	require.NotNil(t, r1)

	r2, err := engine.GetAnalysis()
	require.NoError(t, err)

	// Second call must return the identical pointer (cache hit).
	assert.Same(t, r1, r2, "expected cached result pointer identity")

	engine.Invalidate()

	r3, err := engine.GetAnalysis()
	require.NoError(t, err)
	require.NotNil(t, r3)

	// After invalidation, a new result must be generated.
	assert.NotSame(t, r1, r3, "expected new result after invalidation")
	assert.NotEmpty(t, r3.Symbols)
}

func TestEngineEmpty(t *testing.T) {
	dir := t.TempDir()
	engine := NewEngine(dir)

	result, err := engine.GetAnalysis()
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Empty(t, result.Symbols)
	assert.Empty(t, result.Dependencies)
	assert.Empty(t, result.Imports)
	assert.Empty(t, result.CallGraph)
}

// ---------------------------------------------------------------------------
// TestDOT
// ---------------------------------------------------------------------------

func TestDepsDot(t *testing.T) {
	result := &AnalysisResult{
		Dependencies: []Dependency{
			{From: "main", To: "github.com/pkg/errors", Kind: "require"},
		},
	}

	out := DepsDot(result)
	s := string(out)

	assert.True(t, strings.HasPrefix(s, "digraph dependencies"), "expected DOT header")
	assert.Contains(t, s, "rankdir=LR;")
	assert.Contains(t, s, `"main" -> "github.com/pkg/errors"`)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(s), "}"), "expected DOT closing brace")
}

func TestCallGraphDot(t *testing.T) {
	result := &AnalysisResult{
		CallGraph: []CallEdge{
			{Caller: ".main", Callee: "hello", File: "main.go", Line: 5},
		},
	}

	out := CallGraphDot(result)
	s := string(out)

	assert.True(t, strings.HasPrefix(s, "digraph call_graph"), "expected DOT header")
	assert.Contains(t, s, "rankdir=LR;")
	assert.Contains(t, s, `".main" -> "hello"`)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(s), "}"), "expected DOT closing brace")
}

func TestImportMapDot(t *testing.T) {
	result := &AnalysisResult{
		Imports: []Dependency{
			{From: ".", To: "fmt", Kind: "import"},
		},
	}

	out := ImportMapDot(result)
	s := string(out)

	assert.True(t, strings.HasPrefix(s, "digraph import_map"), "expected DOT header")
	assert.Contains(t, s, "rankdir=LR;")
	assert.Contains(t, s, `"." -> "fmt"`)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(s), "}"), "expected DOT closing brace")
}
