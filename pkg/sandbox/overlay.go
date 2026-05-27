package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateWorkspace copies the entire project directory into workspace.
// This replaces the overlay mount with a pure filesystem copy — no root required.
func CreateWorkspace(projectRoot, workspace string) error {
	if err := os.MkdirAll(filepath.Dir(workspace), 0755); err != nil {
		return fmt.Errorf("mkdir parent: %w", err)
	}
	return copyDir(projectRoot, workspace)
}

// RemoveWorkspace deletes the workspace directory tree.
func RemoveWorkspace(workspace string) error {
	return os.RemoveAll(workspace)
}
