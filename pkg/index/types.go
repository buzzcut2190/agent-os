package index

import "time"

// SymbolKind classifies a code symbol by its syntactic role.
type SymbolKind string

const (
	KindFunction  SymbolKind = "function"
	KindMethod    SymbolKind = "method"
	KindClass     SymbolKind = "class"
	KindInterface SymbolKind = "interface"
	KindVariable  SymbolKind = "variable"
	KindConstant  SymbolKind = "constant"
	KindImport    SymbolKind = "import"
	KindType      SymbolKind = "type"
)

// Position records a source location within a file.
type Position struct {
	File   string
	Line   int
	Column int
}

// Symbol represents a named code entity such as a function, type, or variable.
type Symbol struct {
	Name      string
	Kind      SymbolKind
	Package   string
	Signature string
	Def       Position
	Refs      []Position
}

// Dependency records a directional dependency between two packages or modules.
type Dependency struct {
	From string
	To   string
	Kind string // "import", "require"
}

// CallEdge records a single caller–callee relationship at a specific source location.
type CallEdge struct {
	Caller string
	Callee string
	File   string
	Line   int
}

// AnalysisResult aggregates all semantic information extracted from a codebase
// for a single language.
type AnalysisResult struct {
	Language     string
	Symbols      []Symbol
	Dependencies []Dependency
	Imports      []Dependency
	CallGraph    []CallEdge
	IndexedAt    time.Time
}

// Analyzer defines the interface that every language-specific analyzer must implement.
type Analyzer interface {
	Analyze(root string) (*AnalysisResult, error)
	SupportedExtensions() []string
}
