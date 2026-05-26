package context

import (
	"fmt"
	"sort"
	"strings"
)

// GenerateSummary creates a full project summary.
func GenerateSummary(rootDir string) (*ProjectSummary, error) {
	engine := NewEngine(rootDir)
	return engine.GetSummary()
}

// FormatMarkdown renders a ProjectSummary as markdown.
func FormatMarkdown(summary *ProjectSummary) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Project: %s\n\n", summary.ProjectName))

	// Tech stack
	b.WriteString("## 技术栈\n")
	for _, t := range summary.Types {
		if t.Language != "" {
			b.WriteString(fmt.Sprintf("- Language: %s", t.Language))
			if t.BuildSystem != "" {
				b.WriteString(fmt.Sprintf(" | Build: %s", t.BuildSystem))
			}
			b.WriteString("\n")
		}
	}
	if len(summary.Dependencies) > 0 {
		b.WriteString("- Dependencies: ")
		b.WriteString(strings.Join(summary.Dependencies, ", "))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Directory tree
	if summary.DirectoryTree != "" {
		b.WriteString("## 目录结构\n")
		b.WriteString("```\n")
		b.WriteString(summary.DirectoryTree)
		b.WriteString("```\n\n")
	}

	// Key files
	if len(summary.KeyFiles) > 0 {
		b.WriteString("## 关键文件\n")
		for _, f := range summary.KeyFiles {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
		b.WriteString("\n")
	}

	// Statistics
	b.WriteString("## 统计\n")
	b.WriteString(fmt.Sprintf("- Total files: %d\n", summary.TotalFiles))

	// Sort by file count descending
	type statEntry struct {
		ext  string
		stat FileStat
	}
	var entries []statEntry
	for ext, stat := range summary.FileStatistics {
		entries = append(entries, statEntry{ext, stat})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].stat.Count > entries[j].stat.Count
	})
	for _, e := range entries {
		b.WriteString(fmt.Sprintf("- %s: %d files (%d lines)\n", e.ext, e.stat.Count, e.stat.Lines))
	}

	return b.String()
}
