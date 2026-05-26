package refactor

import "github.com/agent-os/agent-os/pkg/index"

// DeadCodeIssue describes a symbol that appears to be unused.
type DeadCodeIssue struct {
	Symbol string // FQN of the dead symbol
	Kind   string // "function", "method", "variable", "type"
	File   string
	Line   int
	Column int
	Reason string // human-readable explanation
}

// ComplexityIssue describes a function whose McCabe complexity exceeds the threshold.
type ComplexityIssue struct {
	Symbol     string // function name (not FQN)
	File       string
	Line       int
	Complexity int
	Threshold  int
	Package    string
}

// Result is returned by refactoring actions.
type Result struct {
	Action  string
	Success bool
	Summary string
	Changed []string
	Error   string
}

// DeadCodeDetector finds symbols that are defined but never referenced.
type DeadCodeDetector interface {
	Detect(analysis *index.AnalysisResult, includeExported bool) ([]DeadCodeIssue, error)
}

// ComplexityCalculator computes McCabe cyclomatic complexity for Go functions.
type ComplexityCalculator interface {
	Calculate(analysisResult *index.AnalysisResult, threshold int) ([]ComplexityIssue, error)
	CalculateFile(filePath string, threshold int) ([]ComplexityIssue, error)
}
