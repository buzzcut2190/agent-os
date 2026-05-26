package index

import (
	"fmt"
	"sync"
	"time"
)

// Engine provides cached semantic analysis of a codebase.
type Engine struct {
	RootDir       string
	CacheDuration time.Duration
	Analyzers     []Analyzer

	mu       sync.RWMutex
	cached   *AnalysisResult
	cachedAt time.Time
}

// NewEngine creates an Engine for the given root directory with the supplied
// analyzers. If no analyzers are provided, a GoAnalyzer is registered by default.
func NewEngine(root string, analyzers ...Analyzer) *Engine {
	if len(analyzers) == 0 {
		analyzers = append(analyzers, NewGoAnalyzer())
	}
	return &Engine{
		RootDir:       root,
		CacheDuration: 30 * time.Second,
		Analyzers:     analyzers,
	}
}

// GetAnalysis returns a cached analysis or generates a new one.
func (e *Engine) GetAnalysis() (*AnalysisResult, error) {
	e.mu.RLock()
	if e.cached != nil && time.Since(e.cachedAt) < e.CacheDuration {
		result := e.cached
		e.mu.RUnlock()
		return result, nil
	}
	e.mu.RUnlock()

	result, err := e.generate()
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	e.cached = result
	e.cachedAt = time.Now()
	e.mu.Unlock()

	return result, nil
}

// Invalidate forces the next GetAnalysis call to regenerate the result.
func (e *Engine) Invalidate() {
	e.mu.Lock()
	e.cached = nil
	e.mu.Unlock()
}

// generate runs every registered analyzer and merges the results.
func (e *Engine) generate() (*AnalysisResult, error) {
	var merged AnalysisResult

	for _, a := range e.Analyzers {
		r, err := a.Analyze(e.RootDir)
		if err != nil {
			return nil, fmt.Errorf("analyzer %T: %w", a, err)
		}
		merged.Symbols = append(merged.Symbols, r.Symbols...)
		merged.Dependencies = append(merged.Dependencies, r.Dependencies...)
		merged.Imports = append(merged.Imports, r.Imports...)
		merged.CallGraph = append(merged.CallGraph, r.CallGraph...)
	}

	// Pick a representative language; the first analyzer that contributes
	// symbols wins.
	for _, a := range e.Analyzers {
		exts := a.SupportedExtensions()
		if len(exts) > 0 {
			merged.Language = exts[0]
			break
		}
	}

	return &merged, nil
}
