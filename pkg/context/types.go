package context

import (
	"sync"
	"time"
)

// ProjectType describes the detected language and framework of a project.
type ProjectType struct {
	Language      string
	Framework     string
	BuildSystem   string
	TestFramework string
	EntryFiles    []string
}

// ProjectSummary contains the generated context information for a project.
type ProjectSummary struct {
	ProjectName    string
	Types          []ProjectType
	DirectoryTree  string
	KeyFiles       []string
	Dependencies   []string
	FileStatistics map[string]FileStat
	TotalFiles     int
}

// FileStat holds file count and line count for a file type.
type FileStat struct {
	Count int
	Lines int
}

// Engine generates project context summaries with caching.
type Engine struct {
	RootDir       string
	CacheDuration time.Duration
	MaxDepth      int
	Exclude       []string

	mu       sync.RWMutex
	cached   *ProjectSummary
	cachedAt time.Time
}
