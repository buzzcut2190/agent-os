package mcp

import (
	"errors"
	"path/filepath"
	"strings"
)

// Config holds the MCP server configuration.
type Config struct {
	ProjectRoot string
	Transport   string // "stdio" or "sse"
	Port        int
	SessionDir  string
	LogLevel    string
}

// ErrInvalidPath is returned when a path escapes the project root.
var ErrInvalidPath = errors.New("path escapes project root")

// SafePath resolves and validates a path within the project root.
// Returns the absolute safe path or an error if the path is invalid.
func SafePath(root, input string) (string, error) {
	if input == "" {
		return "", errors.New("empty path")
	}

	// Resolve relative to project root
	abs := input
	if !filepath.IsAbs(input) {
		abs = filepath.Join(root, input)
	}
	abs = filepath.Clean(abs)

	// Check that the resolved path is within root
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", ErrInvalidPath
	}
	if strings.HasPrefix(rel, "..") {
		return "", ErrInvalidPath
	}

	// Reject paths starting with "." or containing ".."
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == ".." {
			return "", ErrInvalidPath
		}
	}

	return abs, nil
}

// FileEntry represents a directory entry for list operations.
type FileEntry struct {
	Name  string `json:"name"`
	Type  string `json:"type"` // "file", "dir", "symlink"
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"`
}

