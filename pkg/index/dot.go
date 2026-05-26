package index

import (
	"fmt"
	"strings"
)

// DepsDot returns a DOT digraph of the external dependencies found in the analysis.
func DepsDot(r *AnalysisResult) []byte {
	var b strings.Builder
	b.WriteString("digraph dependencies {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=box, style=filled, fillcolor=lightyellow];\n\n")

	seen := make(map[string]bool)
	for _, d := range r.Dependencies {
		key := d.From + "|" + d.To
		if seen[key] {
			continue
		}
		seen[key] = true
		b.WriteString(fmt.Sprintf("  %q -> %q;\n", sanitizeID(d.From), sanitizeID(d.To)))
	}

	b.WriteString("}\n")
	return []byte(b.String())
}

// CallGraphDot returns a DOT digraph of the call graph.
func CallGraphDot(r *AnalysisResult) []byte {
	var b strings.Builder
	b.WriteString("digraph call_graph {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=ellipse, style=filled, fillcolor=lightblue];\n\n")

	seen := make(map[string]bool)
	for _, c := range r.CallGraph {
		key := c.Caller + "|" + c.Callee
		if seen[key] {
			continue
		}
		seen[key] = true
		b.WriteString(fmt.Sprintf("  %q -> %q;\n", sanitizeID(c.Caller), sanitizeID(c.Callee)))
	}

	b.WriteString("}\n")
	return []byte(b.String())
}

// ImportMapDot returns a DOT digraph of the import relationships between packages.
func ImportMapDot(r *AnalysisResult) []byte {
	var b strings.Builder
	b.WriteString("digraph import_map {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=folder, style=filled, fillcolor=lightgreen];\n\n")

	seen := make(map[string]bool)
	for _, imp := range r.Imports {
		key := imp.From + "|" + imp.To
		if seen[key] {
			continue
		}
		seen[key] = true
		b.WriteString(fmt.Sprintf("  %q -> %q;\n", sanitizeID(imp.From), sanitizeID(imp.To)))
	}

	b.WriteString("}\n")
	return []byte(b.String())
}

// sanitizeID replaces characters that are problematic in DOT identifiers.
func sanitizeID(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
