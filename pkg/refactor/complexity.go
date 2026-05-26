package refactor

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/agent-os/agent-os/pkg/index"
)

// DefaultComplexityThreshold is the McCabe score above which a function is
// flagged for review.
const DefaultComplexityThreshold = 10

// GoComplexityCalculator computes McCabe cyclomatic complexity for Go source
// files using textual analysis (no AST).
type GoComplexityCalculator struct{}

// NewComplexityCalculator creates a new Go complexity calculator.
func NewComplexityCalculator() *GoComplexityCalculator {
	return &GoComplexityCalculator{}
}

// Calculate examines every function/method listed in the analysis result,
// computes its McCabe complexity, and returns those whose score exceeds
// the given threshold.
func (c *GoComplexityCalculator) Calculate(analysisResult *index.AnalysisResult, threshold int) ([]ComplexityIssue, error) {
	if threshold <= 0 {
		threshold = DefaultComplexityThreshold
	}
	if analysisResult == nil {
		return nil, nil
	}

	// Per-file cache of complexity results.
	fileCache := make(map[string][]funcResult)

	var issues []ComplexityIssue
	for _, sym := range analysisResult.Symbols {
		if sym.Kind != index.KindFunction && sym.Kind != index.KindMethod {
			continue
		}
		short := shortSymbolName(sym.Name)

		results, ok := fileCache[sym.Def.File]
		if !ok {
			var err error
			results, err = calcFileComplexity(sym.Def.File)
			if err != nil {
				// Skip files we cannot read; the caller can fall back to
				// CalculateFile for a per-file report.
				continue
			}
			fileCache[sym.Def.File] = results
		}

		for _, r := range results {
			if r.name == short && r.complexity > threshold {
				issues = append(issues, ComplexityIssue{
					Symbol:     sym.Name,
					File:       sym.Def.File,
					Line:       sym.Def.Line,
					Complexity: r.complexity,
					Threshold:  threshold,
					Package:    sym.Package,
				})
				break
			}
		}
	}

	return issues, nil
}

// CalculateFile computes complexity for every function found in a single Go
// source file and returns those whose score exceeds the threshold.
func (c *GoComplexityCalculator) CalculateFile(filePath string, threshold int) ([]ComplexityIssue, error) {
	if threshold <= 0 {
		threshold = DefaultComplexityThreshold
	}

	results, err := calcFileComplexity(filePath)
	if err != nil {
		return nil, fmt.Errorf("calculate complexity for %s: %w", filePath, err)
	}

	issues := make([]ComplexityIssue, 0, len(results))
	for _, r := range results {
		if r.complexity <= threshold {
			continue
		}
		issues = append(issues, ComplexityIssue{
			Symbol:     r.name,
			File:       filePath,
			Line:       r.line,
			Complexity: r.complexity,
			Threshold:  threshold,
		})
	}
	return issues, nil
}

// funcResult holds the computed complexity for one function.
type funcResult struct {
	name       string
	line       int
	complexity int
}

// calcFileComplexity reads a Go source file and computes McCabe complexity for
// every top-level function definition found.
func calcFileComplexity(filePath string) ([]funcResult, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	text := string(src)

	funcs := findGoFunctions(text)
	var results []funcResult
	for _, f := range funcs {
		c := 1 + countDecisionPoints(f.body)
		results = append(results, funcResult{
			name:       f.name,
			line:       f.line,
			complexity: c,
		})
	}
	return results, nil
}

// goFunc describes a single function extracted from source text.
type goFunc struct {
	name string
	line int
	body string // the text between '{' and the matching '}'
}

// findGoFunctions scans Go source for top-level function declarations and
// extracts each function body via brace matching.
func findGoFunctions(src string) []goFunc {
	var funcs []goFunc
	i := 0
	n := len(src)
	line := 1

	for i < n {
		// Advance to next "func" keyword at the start of a line or after
		// whitespace-only characters preceding it.
		funcIdx := strings.Index(src[i:], "func ")
		if funcIdx == -1 {
			// Also try "func(" for receiver-less declarations like func().
			funcIdx = strings.Index(src[i:], "func(")
		}
		if funcIdx == -1 {
			break
		}
		pos := i + funcIdx
		// Count newlines from i to pos to update line number.
		line += strings.Count(src[i:pos], "\n")

		// The keyword "func" may appear inside a string literal, comment, or
		// as part of a longer identifier.  We do a best-effort check: the
		// "func" must be at the beginning of a logical line (preceded only by
		// whitespace or nothing).
		lineStart := lastNewline(src[:pos+1])
		prefix := src[lineStart:pos]
		if !isOnlyWhitespace(prefix) {
			i = pos + 5 // skip "func "
			continue
		}

		// Find the opening brace of the function body.
		bodyStart := strings.IndexByte(src[pos:], '{')
		if bodyStart == -1 {
			i = pos + 5
			continue
		}
		bodyStart = pos + bodyStart

		// Match braces to find the end of the function body.
		bodyEnd, ok := matchBrace(src, bodyStart)
		if !ok {
			i = pos + 5
			continue
		}

		// Extract function name.
		name := extractFuncName(src[pos : bodyStart+1])
		if name == "" {
			i = pos + 5
			continue
		}

		body := src[bodyStart+1 : bodyEnd]
		funcs = append(funcs, goFunc{
			name: name,
			line: line,
			body: body,
		})

		i = bodyEnd + 1
	}

	return funcs
}

// extractFuncName tries to extract the function name from the text between
// "func" and the opening '{'.  It handles both "func Foo(" and
// "func (r *T) Foo(" forms.
func extractFuncName(signature string) string {
	sig := strings.TrimPrefix(signature, "func")
	sig = strings.TrimSpace(sig)

	// If there's a receiver, skip past it.
	if strings.HasPrefix(sig, "(") {
		closeParen := strings.IndexByte(sig, ')')
		if closeParen == -1 {
			return ""
		}
		sig = sig[closeParen+1:]
		sig = strings.TrimSpace(sig)
	}

	// The next token is the function name.
	// Cut at '(' (parameter list) or '{' (body).
	if idx := strings.IndexAny(sig, "({"); idx >= 0 {
		sig = sig[:idx]
	}
	return strings.TrimSpace(sig)
}

// countDecisionPoints counts McCabe decision points in a function body.
// Decision points: if/for/range keywords, case/default in switches,
// and the logical operators && and ||.
func countDecisionPoints(body string) int {
	// Use a simple scanner that skips strings and comments.
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Split(bufio.ScanWords)

	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		tok := scanner.Text()
		switch tok {
		case "if", "for":
			count++
		case "case", "default":
			count++
		case "&&", "||":
			count++
		}
	}
	return count
}

// matchBrace finds the matching '}' for the '{' at position open in src.
func matchBrace(src string, open int) (int, bool) {
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

// lastNewline returns the index just after the last '\n' in s, or 0 if none.
func lastNewline(s string) int {
	idx := strings.LastIndexByte(s, '\n')
	if idx < 0 {
		return 0
	}
	return idx + 1
}

// isOnlyWhitespace reports whether s consists entirely of whitespace runes.
func isOnlyWhitespace(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
