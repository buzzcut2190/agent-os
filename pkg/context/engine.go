package context

import (
	"io/fs"
	"path/filepath"
	"sync"
	"time"
)

// NewEngine creates a context engine for the given root directory.
func NewEngine(rootDir string) *Engine {
	return &Engine{
		RootDir:       rootDir,
		CacheDuration: 30 * time.Second,
		MaxDepth:      3,
		Exclude:       []string{".git", "node_modules", ".agentfs", "vendor", "__pycache__", ".cache", "dist", "build", "target"},
	}
}

// GetSummary returns a cached summary or generates a new one.
func (e *Engine) GetSummary() (*ProjectSummary, error) {
	e.mu.RLock()
	if e.cached != nil && time.Since(e.cachedAt) < e.CacheDuration {
		result := e.cached
		e.mu.RUnlock()
		return result, nil
	}
	e.mu.RUnlock()

	summary, err := e.generate()
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	e.cached = summary
	e.cachedAt = time.Now()
	e.mu.Unlock()

	return summary, nil
}

// Invalidate forces the next GetSummary to regenerate.
func (e *Engine) Invalidate() {
	e.mu.Lock()
	e.cached = nil
	e.mu.Unlock()
}

func (e *Engine) generate() (*ProjectSummary, error) {
	ignorePatterns := loadGitignore(e.RootDir)
	ignorePatterns = append(ignorePatterns, e.Exclude...)

	summary := &ProjectSummary{
		ProjectName:    filepath.Base(e.RootDir),
		Types:          DetectLanguages(e.RootDir),
		FileStatistics: make(map[string]FileStat),
	}

	var allFiles []string
	var mu sync.Mutex

	err := filepath.WalkDir(e.RootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(e.RootDir, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			return nil
		}

		if isIgnored(rel, d.IsDir(), ignorePatterns) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if !d.IsDir() {
			mu.Lock()
			allFiles = append(allFiles, rel)
			mu.Unlock()

			ext := fileType(rel)
			if _, err := d.Info(); err != nil {
				return nil
			}
			s := summary.FileStatistics[ext]
			s.Count++
			s.Lines += countLines(path)
			summary.FileStatistics[ext] = s
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	summary.TotalFiles = len(allFiles)
	summary.DirectoryTree = buildTree(e.RootDir, e.MaxDepth, ignorePatterns)
	summary.KeyFiles = findKeyFiles(e.RootDir)
	summary.Dependencies = extractDependencies(e.RootDir)

	return summary, nil
}
