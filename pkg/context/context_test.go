package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Go project files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.22\n\nrequire github.com/foo/bar v1.0.0\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.sum"), []byte("hash1\nhash2\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Makefile"), []byte("build:\n\tgo build ./...\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Project\n"), 0644))

	// Subdirectory with files
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg", "lib"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pkg", "lib", "lib.go"), []byte("package lib\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pkg", "lib", "lib_test.go"), []byte("package lib\n"), 0644))

	// Ignored directory
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor", "dep.go"), []byte("package dep\n"), 0644))

	return dir
}

func TestDetectLanguage(t *testing.T) {
	dir := setupTestProject(t)
	types := DetectLanguages(dir)
	require.NotEmpty(t, types)

	lang := types[0].Language
	assert.Equal(t, "Go", lang)
	assert.Equal(t, "Go Modules", types[0].BuildSystem)
}

func TestGenerateSummary(t *testing.T) {
	dir := setupTestProject(t)
	summary, err := GenerateSummary(dir)
	require.NoError(t, err)
	require.NotNil(t, summary)

	assert.Equal(t, filepath.Base(dir), summary.ProjectName)
	assert.True(t, summary.TotalFiles > 0, "should count files")
	assert.NotEmpty(t, summary.DirectoryTree, "should have dir tree")
	assert.NotEmpty(t, summary.KeyFiles, "should have key files")
	assert.NotEmpty(t, summary.Dependencies, "should have dependencies")
	assert.NotEmpty(t, summary.FileStatistics, "should have file stats")
}

func TestFormatMarkdown(t *testing.T) {
	dir := setupTestProject(t)
	summary, err := GenerateSummary(dir)
	require.NoError(t, err)

	md := FormatMarkdown(summary)
	assert.Contains(t, md, "技术栈")
	assert.Contains(t, md, "目录结构")
	assert.Contains(t, md, "关键文件")
	assert.Contains(t, md, "统计")
	assert.Contains(t, md, summary.ProjectName)
}

func TestIgnoreRules(t *testing.T) {
	dir := setupTestProject(t)

	// Write .gitignore
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("vendor/\n*.log\n"), 0644))

	summary, err := GenerateSummary(dir)
	require.NoError(t, err)

	// Tree should not contain vendor
	assert.NotContains(t, summary.DirectoryTree, "vendor", ".gitignore dirs should be excluded")

	// Verify total file count excludes vendor files
	// vendor/ contains dep.go which should be excluded
	assert.NotContains(t, summary.DirectoryTree, "dep.go")
}

func TestCacheInvalidation(t *testing.T) {
	dir := setupTestProject(t)

	engine := NewEngine(dir)
	engine.CacheDuration = 0 // disable caching

	s1, err := engine.GetSummary()
	require.NoError(t, err)

	// Add a new file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "newfile.go"), []byte("package main\n"), 0644))

	engine.Invalidate()
	s2, err := engine.GetSummary()
	require.NoError(t, err)

	assert.Greater(t, s2.TotalFiles, s1.TotalFiles, "should detect new file after invalidation")
}

func TestCacheHit(t *testing.T) {
	dir := setupTestProject(t)

	engine := NewEngine(dir)
	engine.CacheDuration = 60 * time.Second

	s1, err := engine.GetSummary()
	require.NoError(t, err)

	// Modify source without invalidating
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hidden.go"), []byte("package main\n"), 0644))

	s2, err := engine.GetSummary()
	require.NoError(t, err)

	assert.Equal(t, s1.TotalFiles, s2.TotalFiles, "should return cached result")
}

func TestDepExtraction(t *testing.T) {
	dir := setupTestProject(t)

	summary, err := GenerateSummary(dir)
	require.NoError(t, err)

	found := false
	for _, dep := range summary.Dependencies {
		if strings.Contains(dep, "github.com/foo/bar") {
			found = true
		}
	}
	assert.True(t, found, "should extract go.mod dependency")
}
