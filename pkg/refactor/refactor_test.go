package refactor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-os/agent-os/pkg/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func makeAnalysisResult() *index.AnalysisResult {
	return &index.AnalysisResult{
		Symbols: []index.Symbol{
			{Name: "test.usedFunc", Kind: index.KindFunction, Def: index.Position{File: "main.go", Line: 5}, Refs: []index.Position{{File: "main.go", Line: 10}}},
			{Name: "test.unusedFunc", Kind: index.KindFunction, Def: index.Position{File: "main.go", Line: 15}, Refs: nil},
			{Name: "test.ExportedFunc", Kind: index.KindFunction, Def: index.Position{File: "main.go", Line: 20}, Refs: nil},
			{Name: "test.main", Kind: index.KindFunction, Def: index.Position{File: "main.go", Line: 1}, Refs: nil},
			{Name: "test.init", Kind: index.KindFunction, Def: index.Position{File: "main.go", Line: 0}, Refs: nil},
			{Name: "test.usedVar", Kind: index.KindVariable, Def: index.Position{File: "main.go", Line: 25}, Refs: []index.Position{{File: "main.go", Line: 30}}},
		},
	}
}

func findDeadIssue(issues []DeadCodeIssue, name string) *DeadCodeIssue {
	for i := range issues {
		if issues[i].Symbol == name {
			return &issues[i]
		}
	}
	return nil
}

func writeTempGo(t *testing.T, name, code string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(code), 0644))
	return path
}

func setupComplexityProject(t *testing.T) string {
	t.Helper()
	code := `package test
func Simple() { return }
func Moderate(a int) {
	if a > 0 {
		println("pos")
	} else {
		println("neg")
	}
}
func Complex(x int) int {
	switch {
	case x > 0 && x < 10:
		if x%2 == 0 { return 1 }
		return 2
	case x >= 10:
		for i := 0; i < x; i++ {
			if i%3 == 0 { println(i) }
		}
		return 3
	}
	return 0
}
`
	return writeTempGo(t, "sample.go", code)
}

// ---------------------------------------------------------------------------
// DeadCodeDetector
// ---------------------------------------------------------------------------

func TestDetectDeadCode(t *testing.T) {
	d := NewDeadCodeDetector()
	issues, err := d.Detect(makeAnalysisResult(), false)
	require.NoError(t, err)

	issue := findDeadIssue(issues, "test.unusedFunc")
	require.NotNil(t, issue, "unexported unreferenced function should be detected")
	assert.Equal(t, "function", issue.Kind)
	assert.Equal(t, "main.go", issue.File)
	assert.Equal(t, 15, issue.Line)
	assert.Contains(t, issue.Reason, "unusedFunc")
}

func TestDetectLivingCode(t *testing.T) {
	d := NewDeadCodeDetector()
	issues, err := d.Detect(makeAnalysisResult(), false)
	require.NoError(t, err)

	assert.Nil(t, findDeadIssue(issues, "test.usedFunc"), "referenced function should not be dead code")
	assert.Nil(t, findDeadIssue(issues, "test.usedVar"), "referenced variable should not be dead code")
}

func TestSkipExported(t *testing.T) {
	d := NewDeadCodeDetector()
	issues, err := d.Detect(makeAnalysisResult(), false)
	require.NoError(t, err)

	assert.Nil(t, findDeadIssue(issues, "test.ExportedFunc"),
		"exported function should be skipped when includeExported=false")
}

func TestSkipEntryPoints(t *testing.T) {
	d := NewDeadCodeDetector()
	issues, err := d.Detect(makeAnalysisResult(), false)
	require.NoError(t, err)

	assert.Nil(t, findDeadIssue(issues, "test.main"), "main should be skipped")
	assert.Nil(t, findDeadIssue(issues, "test.init"), "init should be skipped")
}

func TestDetectAllDeadCode(t *testing.T) {
	d := NewDeadCodeDetector()
	issues, err := d.Detect(makeAnalysisResult(), true)
	require.NoError(t, err)

	issue := findDeadIssue(issues, "test.ExportedFunc")
	require.NotNil(t, issue, "exported dead code should be reported when includeExported=true")
	assert.Contains(t, issue.Reason, "exported")

	// Entry points are always excluded.
	assert.Nil(t, findDeadIssue(issues, "test.main"))
	assert.Nil(t, findDeadIssue(issues, "test.init"))

	// Unused unexported is still reported.
	assert.NotNil(t, findDeadIssue(issues, "test.unusedFunc"))
}

// ---------------------------------------------------------------------------
// ComplexityCalculator
// ---------------------------------------------------------------------------

func TestComplexitySimple(t *testing.T) {
	path := writeTempGo(t, "simple.go", "package p\nfunc Simple() { return }\n")
	c := NewComplexityCalculator()

	// Complexity is 1.  Default threshold (10) leaves it unfiltered.
	issues, err := c.CalculateFile(path, 0)
	require.NoError(t, err)
	assert.Empty(t, issues, "Simple (complexity=1) should be below the default threshold of 10")
}

func TestComplexityModerate(t *testing.T) {
	path := writeTempGo(t, "moderate.go", `package p
func Moderate(a int) {
	if a > 0 { println("pos") } else { println("neg") }
}
`)
	c := NewComplexityCalculator()

	// Complexity is 2 (1 base + 1 if).  Flagged when threshold=1.
	issues, err := c.CalculateFile(path, 1)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, 2, issues[0].Complexity)
	assert.Equal(t, "Moderate", issues[0].Symbol)

	// Excluded when threshold reaches 2 (2 is not > 2).
	issues, err = c.CalculateFile(path, 2)
	require.NoError(t, err)
	assert.Empty(t, issues)
}

func TestComplexityComplex(t *testing.T) {
	path := writeTempGo(t, "complex.go", `package p
func Complex(x int) int {
	switch {
	case x > 0 && x < 10:
		if x%2 == 0 { return 1 }
		return 2
	case x >= 10:
		for i := 0; i < x; i++ {
			if i%3 == 0 { println(i) }
		}
		return 3
	}
	return 0
}
`)
	c := NewComplexityCalculator()

	issues, err := c.CalculateFile(path, 1)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.GreaterOrEqual(t, issues[0].Complexity, 4, "switch with cases, &&, if, for should score >= 4")
	assert.Equal(t, "Complex", issues[0].Symbol)
}

func TestComplexityThreshold(t *testing.T) {
	path := setupComplexityProject(t) // Simple(1), Moderate(2), Complex(7)
	c := NewComplexityCalculator()

	// Threshold 1: only Simple (1 not > 1) is excluded.
	issues, err := c.CalculateFile(path, 1)
	require.NoError(t, err)
	require.Len(t, issues, 2)
	assert.Equal(t, "Moderate", issues[0].Symbol)
	assert.Equal(t, "Complex", issues[1].Symbol)

	// Threshold 6: only Complex (7 > 6) survives.
	issues, err = c.CalculateFile(path, 6)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "Complex", issues[0].Symbol)
}

func TestCalculateFile(t *testing.T) {
	path := setupComplexityProject(t)
	c := NewComplexityCalculator()

	// Threshold 1 returns Moderate and Complex.
	issues, err := c.CalculateFile(path, 1)
	require.NoError(t, err)
	assert.Len(t, issues, 2)

	for _, iss := range issues {
		assert.NotEmpty(t, iss.Symbol)
		assert.Equal(t, path, iss.File)
		assert.Greater(t, iss.Complexity, 0)
		assert.Equal(t, 1, iss.Threshold)
	}
}
