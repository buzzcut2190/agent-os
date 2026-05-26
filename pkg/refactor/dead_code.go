package refactor

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/agent-os/agent-os/pkg/index"
)

// GoDeadCodeDetector detects unused symbols in Go codebases.
type GoDeadCodeDetector struct{}

// NewDeadCodeDetector creates a new Go dead code detector.
func NewDeadCodeDetector() *GoDeadCodeDetector {
	return &GoDeadCodeDetector{}
}

// entrypointNames lists function names that are expected to have zero references
// because they are invoked by the runtime or toolchain rather than by user code.
var entrypointNames = map[string]bool{
	"init": true,
	"main": true,
}

// Detect scans an analysis result and returns every symbol whose reference
// list is empty.  Exported symbols are skipped when includeExported is false.
// The well-known entry points init and main are always excluded.
func (d *GoDeadCodeDetector) Detect(analysis *index.AnalysisResult, includeExported bool) ([]DeadCodeIssue, error) {
	if analysis == nil {
		return nil, nil
	}

	var issues []DeadCodeIssue
	for _, sym := range analysis.Symbols {
		if len(sym.Refs) > 0 {
			continue
		}

		shortName := shortSymbolName(sym.Name)
		if entrypointNames[shortName] {
			continue
		}

		exported := isExportedGoName(shortName)
		if exported && !includeExported {
			continue
		}

		reason := buildDeadCodeReason(shortName, exported, includeExported)
		issues = append(issues, DeadCodeIssue{
			Symbol: sym.Name,
			Kind:   string(sym.Kind),
			File:   sym.Def.File,
			Line:   sym.Def.Line,
			Column: sym.Def.Column,
			Reason: reason,
		})
	}

	return issues, nil
}

// shortSymbolName extracts the leaf name from a fully-qualified symbol.
// For "pkg/fs.OpenDir" it returns "OpenDir"; for "fmt.Println" it returns "Println".
func shortSymbolName(fqn string) string {
	if idx := strings.LastIndex(fqn, "."); idx >= 0 {
		return fqn[idx+1:]
	}
	// Fallback: last path segment.
	if idx := strings.LastIndex(fqn, "/"); idx >= 0 {
		return fqn[idx+1:]
	}
	return fqn
}

// isExportedGoName reports whether name is an exported Go identifier (first
// rune is an ASCII uppercase letter or any Unicode upper-case letter).
func isExportedGoName(name string) bool {
	if name == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}

// buildDeadCodeReason constructs a human-readable reason for why a symbol is
// reported as dead code.
func buildDeadCodeReason(name string, exported bool, includeExported bool) string {
	if exported {
		return fmt.Sprintf("exported symbol %q has no references (includeExported=%v)", name, includeExported)
	}
	return fmt.Sprintf("unexported symbol %q has no references anywhere in the codebase", name)
}
