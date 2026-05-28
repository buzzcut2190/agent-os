package sandbox

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// NewManager creates a session manager.
func NewManager(baseDir string) *Manager {
	return &Manager{BaseDir: baseDir}
}

// DefaultBaseDir returns the default session storage path.
func DefaultBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "agentfs-sessions")
	}
	return filepath.Join(home, ".local", "share", "agentfs", "sessions")
}

// StartSession creates a new copy-based isolated session for the given project.
func (m *Manager) StartSession(projectRoot string) (*Session, error) {
	id := newSessionID()
	sessDir := filepath.Join(m.BaseDir, id)
	workspace := filepath.Join(sessDir, "workspace")

	if err := os.MkdirAll(sessDir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	absProject, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}

	sess := &Session{
		ID:        id,
		Created:   time.Now(),
		Project:   absProject,
		Workspace: workspace,
		Status:    StatusActive,
	}

	if err := CreateWorkspace(absProject, sess.Workspace); err != nil {
		os.RemoveAll(sessDir)
		return nil, err
	}

	if err := saveSession(m.sessionPath(id), sess); err != nil {
		os.RemoveAll(sessDir)
		return nil, err
	}

	return sess, nil
}

// CommitSession applies all changes from the workspace back to the project.
func (m *Manager) CommitSession(id string) error {
	sess, err := loadSession(m.sessionPath(id))
	if err != nil {
		return err
	}
	if sess.Status != StatusActive {
		return fmt.Errorf("session %s is not active (status: %s)", id, sess.Status)
	}

	// Sync changes from workspace to project
	if err := syncWorkspaceToProject(sess.Workspace, sess.Project); err != nil {
		return fmt.Errorf("sync changes: %w", err)
	}

	// Clean up workspace (keep session dir for metadata)
	os.RemoveAll(sess.Workspace)

	// Update status
	sess.Status = StatusCommitted
	return saveSession(m.sessionPath(id), sess)
}

// DiscardSession discards all changes and cleans up.
func (m *Manager) DiscardSession(id string) error {
	sess, err := loadSession(m.sessionPath(id))
	if err != nil {
		return err
	}
	if sess.Status != StatusActive {
		return fmt.Errorf("session %s is not active (status: %s)", id, sess.Status)
	}

	// Delete workspace (discard all changes; keep session dir for metadata)
	os.RemoveAll(sess.Workspace)

	// Update status
	sess.Status = StatusDiscarded
	return saveSession(m.sessionPath(id), sess)
}

// ListSessions returns all sessions.
func (m *Manager) ListSessions() ([]*Session, error) {
	if err := os.MkdirAll(m.BaseDir, 0755); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(m.BaseDir)
	if err != nil {
		return nil, err
	}

	var sessions []*Session
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sess, err := loadSession(filepath.Join(m.BaseDir, e.Name(), "session.json"))
		if err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// GetSession loads a session by ID.
func (m *Manager) GetSession(id string) (*Session, error) {
	return loadSession(m.sessionPath(id))
}

func (m *Manager) sessionPath(id string) string {
	return filepath.Join(m.BaseDir, id, "session.json")
}

func newSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func saveSession(path string, sess *Session) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func loadSession(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sess := &Session{}
	if err := json.Unmarshal(data, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// syncWorkspaceToProject copies all changes from the workspace back to the
// original project directory. Files present in the project but missing from
// the workspace are treated as deleted.
func syncWorkspaceToProject(workspace, project string) error {
	// Build a set of all relative paths in the workspace
	wsFiles := make(map[string]bool)
	err := filepath.WalkDir(workspace, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil || rel == "." {
			return err
		}
		wsFiles[rel] = d.IsDir()
		return nil
	})
	if err != nil {
		return err
	}

	// Walk the project and delete files/dirs that no longer exist in the workspace
	err = filepath.WalkDir(project, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(project, path)
		if err != nil || rel == "." {
			return err
		}
		if _, exists := wsFiles[rel]; !exists {
			if d.IsDir() {
				return os.RemoveAll(path)
			}
			return os.Remove(path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Copy all workspace files to project (handles new and modified files)
	return filepath.WalkDir(workspace, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil || rel == "." {
			return err
		}
		target := filepath.Join(project, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()

	_, err = io.Copy(d, s)
	if err != nil {
		return err
	}

	// Copy permissions
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}

// skipDirs are directory names that should never be copied into a session workspace.
var skipDirs = map[string]bool{
	".agentfs": true, // FUSE mount points and metadata
	".git":    true, // version control (preserved in project, not needed in sandbox)
	"node_modules": true, // dependency caches (too large, can be restored via package manager)
	".venv":   true, // Python virtual environments
	"target":  true, // Rust/Cargo build artifacts
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if skipDirs[e.Name()] {
			continue
		}
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// DiffSession walks the workspace and compares against the original project.
// It returns a list of changes: added, modified, and deleted files.
func (m *Manager) DiffSession(id string) ([]DiffEntry, error) {
	sess, err := loadSession(m.sessionPath(id))
	if err != nil {
		return nil, err
	}
	return diffSessionFiles(sess)
}

// filesEqual returns true when both files have identical content (SHA-256).
func filesEqual(a, b string) (bool, error) {
	fa, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer fa.Close()
	fb, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer fb.Close()
	ha, hb := sha256.New(), sha256.New()
	if _, err := io.Copy(ha, fa); err != nil {
		return false, err
	}
	if _, err := io.Copy(hb, fb); err != nil {
		return false, err
	}
	return bytes.Equal(ha.Sum(nil), hb.Sum(nil)), nil
}

// diffSessionFiles is the internal implementation that compares workspace vs project.
func diffSessionFiles(sess *Session) ([]DiffEntry, error) {
	var changes []DiffEntry

	err := filepath.Walk(sess.Workspace, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sess.Workspace, path)
		if err != nil || rel == "." {
			return err
		}
		if info.IsDir() {
			return nil
		}
		projectPath := filepath.Join(sess.Project, rel)
		projectInfo, projectErr := os.Stat(projectPath)
		if projectErr != nil {
			if os.IsNotExist(projectErr) {
				changes = append(changes, DiffEntry{Path: rel, Status: "added"})
			}
			return nil
		}
		if info.Size() == projectInfo.Size() {
			if equal, _ := filesEqual(projectPath, path); equal {
				return nil
			}
		}
		changes = append(changes, DiffEntry{Path: rel, Status: "modified"})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Detect deleted files: files in project but not in workspace
	err = filepath.Walk(sess.Project, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable dirs (e.g. stale FUSE mounts)
		}
		rel, err := filepath.Rel(sess.Project, path)
		if err != nil || rel == "." {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		wsPath := filepath.Join(sess.Workspace, rel)
		if _, err := os.Stat(wsPath); os.IsNotExist(err) {
			changes = append(changes, DiffEntry{Path: rel, Status: "deleted"})
		}
		return nil
	})
	return changes, err
}
