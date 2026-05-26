package index

import (
	"fmt"
	"go/parser"
	"go/token"
	"path/filepath"
	"time"
)

// GoAnalyzer extracts semantic information from Go source files.
type GoAnalyzer struct{}

// NewGoAnalyzer creates a standard Go analyzer.
func NewGoAnalyzer() *GoAnalyzer {
	return &GoAnalyzer{}
}

// SupportedExtensions reports the file extensions this analyzer handles.
func (a *GoAnalyzer) SupportedExtensions() []string {
	return []string{".go"}
}

// Analyze parses every Go file under root and returns a combined AnalysisResult.
// Files with syntax errors are skipped gracefully.
func (a *GoAnalyzer) Analyze(root string) (*AnalysisResult, error) {
	files, err := discoverGoFiles(root)
	if err != nil {
		return nil, fmt.Errorf("discover go files: %w", err)
	}

	result := &AnalysisResult{
		Language:  "go",
		IndexedAt: time.Now(),
	}

	for _, fname := range files {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, fname, nil, parser.ParseComments)
		if err != nil {
			// Syntax errors: skip gracefully, continue with remaining files.
			continue
		}

		rel, err := filepath.Rel(root, fname)
		if err != nil {
			rel = fname
		}
		pkgDir := filepath.Dir(rel)
		pkgName := f.Name.Name

		symbols := extractSymbols(rel, pkgDir, pkgName, fset, f)
		imports, deps := extractImportsAndDeps(rel, pkgDir, pkgName, f)
		edges := extractCallEdges(rel, pkgDir, pkgName, fset, f)

		result.Symbols = append(result.Symbols, symbols...)
		result.Imports = append(result.Imports, imports...)
		result.Dependencies = append(result.Dependencies, deps...)
		result.CallGraph = append(result.CallGraph, edges...)
	}

	return result, nil
}
