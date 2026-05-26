package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-os/agent-os/pkg/index"
	"github.com/agent-os/agent-os/pkg/refactor"
)

// ---------------------------------------------------------------------------
// Report generators for the @refactor/* virtual files
// ---------------------------------------------------------------------------

// deadCodeReport returns a human-readable dead-code report.
func (fs *FileSystem) deadCodeReport() ([]byte, error) {
	analysis := fs.buildAnalysisResult()
	eng := fs.refactorEngineLazy()

	issues, err := eng.deadCode.Detect(analysis, false)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString("# Dead Code Report\n\n")
	if len(issues) == 0 {
		b.WriteString("No dead code found.\n")
	} else {
		fmt.Fprintf(&b, "Found %d potentially unused symbols:\n\n", len(issues))
		for _, issue := range issues {
			fmt.Fprintf(&b, "- **%s** (%s) at %s:%d:%d\n",
				issue.Symbol, issue.Kind, issue.File, issue.Line, issue.Column)
			fmt.Fprintf(&b, "  %s\n\n", issue.Reason)
		}
	}
	return []byte(b.String()), nil
}

// complexityReport returns a human-readable complexity report.
func (fs *FileSystem) complexityReport() ([]byte, error) {
	analysis := fs.buildAnalysisResult()
	eng := fs.refactorEngineLazy()

	issues, err := eng.complexity.Calculate(analysis, refactor.DefaultComplexityThreshold)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString("# Complexity Report\n\n")
	fmt.Fprintf(&b, "Threshold: %d (McCabe cyclomatic complexity)\n\n",
		refactor.DefaultComplexityThreshold)

	if len(issues) == 0 {
		b.WriteString("All functions are within the complexity threshold.\n")
	} else {
		fmt.Fprintf(&b, "Found %d functions exceeding the threshold:\n\n", len(issues))
		for _, issue := range issues {
			fmt.Fprintf(&b, "- **%s** at %s:%d  (complexity: %d, threshold: %d)\n",
				issue.Symbol, issue.File, issue.Line, issue.Complexity, issue.Threshold)
		}
	}
	return []byte(b.String()), nil
}

// lintReport returns basic code-quality suggestions.
func (fs *FileSystem) lintReport() ([]byte, error) {
	analysis := fs.buildAnalysisResult()

	type lintIssue struct {
		file    string
		message string
	}

	var issues []lintIssue
	seen := make(map[string]bool)

	for _, sym := range analysis.Symbols {
		if seen[sym.Def.File] {
			continue
		}
		seen[sym.Def.File] = true

		fi, err := os.Stat(sym.Def.File)
		if err != nil {
			continue
		}
		sizeKB := fi.Size() / 1024
		if sizeKB > 50 {
			issues = append(issues, lintIssue{
				file:    sym.Def.File,
				message: fmt.Sprintf("file is large (%d KB); consider splitting", sizeKB),
			})
		}

		// Check nesting depth with a quick heuristic.
		depth := estimateMaxNesting(sym.Def.File)
		if depth > 5 {
			issues = append(issues, lintIssue{
				file:    sym.Def.File,
				message: fmt.Sprintf("deeply nested (max depth ~%d); consider refactoring", depth),
			})
		}
	}

	var b strings.Builder
	b.WriteString("# Lint Report\n\n")
	if len(issues) == 0 {
		b.WriteString("No lint issues found.\n")
	} else {
		fmt.Fprintf(&b, "Found %d issues:\n\n", len(issues))
		for _, issue := range issues {
			fmt.Fprintf(&b, "- %s: %s\n", issue.file, issue.message)
		}
	}
	return []byte(b.String()), nil
}

// estimateMaxNesting computes an approximate maximum brace-nesting depth for
// a Go source file by scanning for '{' and '}' characters.
func estimateMaxNesting(filePath string) int {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}
	depth, maxDepth := 0, 0
	for _, ch := range string(src) {
		switch ch {
		case '{':
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
		case '}':
			depth--
		}
	}
	return maxDepth
}

// ---------------------------------------------------------------------------
// Lightweight analysis builder (replaced by Module 0 indexer later)
// ---------------------------------------------------------------------------

// buildAnalysisResult produces a basic AnalysisResult by walking the source
// directory tree.  This is a lightweight fallback; the full indexer from
// Module 0 will eventually replace it.
func (fs *FileSystem) buildAnalysisResult() *index.AnalysisResult {
	result := &index.AnalysisResult{
		Language: "go",
	}

	_ = filepath.WalkDir(fs.sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, _ := filepath.Rel(fs.sourceDir, path)
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		text := string(src)

		funcs := findGoFuncRefs(text)
		for _, f := range funcs {
			pkg := filepath.Dir(rel)
			fqn := pkg + "." + f.name
			result.Symbols = append(result.Symbols, index.Symbol{
				Name:      fqn,
				Kind:      f.kind,
				Package:   pkg,
				Signature: f.signature,
				Def: index.Position{
					File:   path,
					Line:   f.line,
					Column: 1,
				},
			})
		}
		return nil
	})

	return result
}

// goFuncRef is a lightweight function reference extracted from source text.
type goFuncRef struct {
	name      string
	kind      index.SymbolKind
	signature string
	line      int
}

// findGoFuncRefs extracts function/method declarations from Go source text.
func findGoFuncRefs(src string) []goFuncRef {
	var refs []goFuncRef
	i := 0
	line := 1

	for i < len(src) {
		funcIdx := strings.Index(src[i:], "func ")
		if funcIdx == -1 {
			funcIdx = strings.Index(src[i:], "func(")
		}
		if funcIdx == -1 {
			break
		}
		pos := i + funcIdx
		line += strings.Count(src[i:pos], "\n")

		// Find '{' after the func keyword.
		bodyStart := strings.IndexByte(src[pos:], '{')
		if bodyStart == -1 {
			i = pos + 5
			continue
		}

		sig := src[pos : pos+bodyStart]
		name := extractDeclName(sig)
		if name == "" {
			i = pos + 5
			continue
		}

		kind := index.KindFunction
		if strings.Contains(sig, ")") && strings.IndexByte(sig, '(') < strings.IndexByte(sig, ')') {
			kind = index.KindMethod // has a receiver
		}

		matchEnd, ok := findMatchingBrace(src, pos+bodyStart)
		if !ok {
			i = pos + 5
			continue
		}

		refs = append(refs, goFuncRef{
			name:      name,
			kind:      kind,
			signature: strings.TrimSpace(sig),
			line:      line,
		})

		i = matchEnd + 1
	}

	return refs
}

// extractDeclName extracts the function name from a declaration prefix such as
// "func Foo(" or "func (r *T) Foo(".
func extractDeclName(sig string) string {
	s := strings.TrimPrefix(sig, "func")
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "(") {
		closeParen := strings.IndexByte(s, ')')
		if closeParen == -1 {
			return ""
		}
		s = s[closeParen+1:]
		s = strings.TrimSpace(s)
	}

	if idx := strings.IndexAny(s, "({"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// findMatchingBrace returns the index of the '}' that matches the '{' at
// position open.
func findMatchingBrace(src string, open int) (int, bool) {
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}
