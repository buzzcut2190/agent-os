package context

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func loadGitignore(rootDir string) []string {
	path := filepath.Join(rootDir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var patterns []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func isIgnored(relPath string, isDir bool, patterns []string) bool {
	parts := strings.Split(relPath, string(filepath.Separator))
	for _, pattern := range patterns {
		clean := strings.TrimSuffix(pattern, "/")
		for _, part := range parts {
			if part == clean {
				return true
			}
		}
		if strings.HasPrefix(pattern, "*.") {
			ext := pattern[1:]
			if strings.HasSuffix(relPath, ext) {
				return true
			}
		}
		if strings.HasSuffix(pattern, "/") && isDir {
			dirName := strings.TrimSuffix(pattern, "/")
			if parts[0] == dirName {
				return true
			}
		}
	}
	return false
}

func fileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "Go"
	case ".py":
		return "Python"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".js", ".jsx":
		return "JavaScript"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".c", ".h":
		return "C"
	case ".cpp", ".hpp", ".cc":
		return "C++"
	case ".rb":
		return "Ruby"
	case ".md":
		return "Markdown"
	case ".yaml", ".yml":
		return "YAML"
	case ".json":
		return "JSON"
	case ".toml":
		return "TOML"
	case ".xml":
		return "XML"
	case ".sh":
		return "Shell"
	case ".mod", ".sum":
		return "Go Module"
	case ".txt":
		return "Text"
	default:
		if ext == "" {
			base := filepath.Base(path)
			switch base {
			case "Makefile", "Dockerfile", "Jenkinsfile":
				return base
			case "go.mod", "go.sum":
				return "Go Module"
			}
			return "Other"
		}
		return strings.TrimPrefix(ext, ".")
	}
}

func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count
}

func buildTree(rootDir string, maxDepth int, ignorePatterns []string) string {
	var b strings.Builder
	b.WriteString(filepath.Base(rootDir) + "/\n")
	buildTreeHelper(&b, rootDir, "", 0, maxDepth, ignorePatterns)
	return b.String()
}

func buildTreeHelper(b *strings.Builder, dir, prefix string, depth, maxDepth int, ignore []string) {
	if depth >= maxDepth {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	filtered := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		if !isIgnored(e.Name(), e.IsDir(), ignore) {
			filtered = append(filtered, e)
		}
	}

	for i, e := range filtered {
		isLast := i == len(filtered)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		b.WriteString(fmt.Sprintf("%s%s%s\n", prefix, connector, name))

		if e.IsDir() {
			childPrefix := prefix + "│   "
			if isLast {
				childPrefix = prefix + "    "
			}
			buildTreeHelper(b, filepath.Join(dir, e.Name()), childPrefix, depth+1, maxDepth, ignore)
		}
	}
}

func findKeyFiles(rootDir string) []string {
	keyNames := []string{
		"README.md", "README", "readme.md",
		"go.mod", "package.json", "Cargo.toml", "requirements.txt", "pyproject.toml",
		"Makefile", "Dockerfile", "docker-compose.yml", "docker-compose.yaml",
		".gitignore", "CLAUDE.md",
	}
	var found []string
	for _, name := range keyNames {
		if _, err := os.Stat(filepath.Join(rootDir, name)); err == nil {
			found = append(found, name)
		}
	}
	return found
}

func extractDependencies(rootDir string) []string {
	var deps []string

	if data, err := os.ReadFile(filepath.Join(rootDir, "go.mod")); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.Contains(line, "github.com/") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "module ") {
				continue
			}
			parts := strings.Fields(line)
			if strings.HasPrefix(line, "require ") && len(parts) >= 2 {
				deps = append(deps, parts[1])
			} else if len(parts) >= 1 && strings.Contains(parts[0], ".") {
				deps = append(deps, parts[0])
			}
		}
	}

	if data, err := os.ReadFile(filepath.Join(rootDir, "package.json")); err == nil {
		for _, section := range []string{`"dependencies"`, `"devDependencies"`} {
			idx := strings.Index(string(data), section)
			if idx < 0 {
				continue
			}
			rest := string(data)[idx:]
			start := strings.Index(rest, "{")
			end := strings.Index(rest, "}")
			if start >= 0 && end > start {
				block := rest[start+1 : end]
				for _, line := range strings.Split(block, ",") {
					line = strings.TrimSpace(line)
					if idx := strings.Index(line, ":"); idx > 0 {
						name := strings.Trim(strings.TrimSpace(line[:idx]), `"`)
						deps = append(deps, name)
					}
				}
			}
		}
	}

	if data, err := os.ReadFile(filepath.Join(rootDir, "requirements.txt")); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				name := strings.Split(line, "==")[0]
				name = strings.Split(name, ">=")[0]
				name = strings.Split(name, "<=")[0]
				name = strings.Split(name, "~=")[0]
				name = strings.TrimSpace(name)
				if name != "" {
					deps = append(deps, name)
				}
			}
		}
	}

	return deps
}
