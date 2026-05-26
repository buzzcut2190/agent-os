package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// StartSession creates a new overlay session for the given project.
func (m *Manager) StartSession(projectRoot string) (*Session, error) {
	id := newSessionID()
	sessDir := filepath.Join(m.BaseDir, id)
	upper := filepath.Join(sessDir, "upper")
	work := filepath.Join(sessDir, "work")
	merged := filepath.Join(sessDir, "merged")

	if err := os.MkdirAll(sessDir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	absProject, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}

	sess := &Session{
		ID:      id,
		Created: time.Now(),
		Project: absProject,
		Lower:   absProject,
		Upper:   upper,
		Work:    work,
		Merged:  merged,
		Status:  StatusActive,
	}

	if err := CreateOverlay(sess.Lower, sess.Upper, sess.Work, sess.Merged); err != nil {
		os.RemoveAll(sessDir)
		return nil, err
	}

	if err := saveSession(m.sessionPath(id), sess); err != nil {
		_ = RemoveOverlay(merged)
		os.RemoveAll(sessDir)
		return nil, err
	}

	return sess, nil
}

// CommitSession applies all changes from the session back to the project.
func (m *Manager) CommitSession(id string) error {
	sess, err := loadSession(m.sessionPath(id))
	if err != nil {
		return err
	}
	if sess.Status != StatusActive {
		return fmt.Errorf("session %s is not active (status: %s)", id, sess.Status)
	}

	// Sync changes from upper to project (lower)
	// We use a file-by-file copy to correctly handle deletions
	if err := syncUpperToLower(sess.Upper, sess.Lower); err != nil {
		return fmt.Errorf("sync changes: %w", err)
	}

	// Unmount overlay
	if err := RemoveOverlay(sess.Merged); err != nil {
		return fmt.Errorf("unmount: %w", err)
	}

	// Clean up session dirs
	os.RemoveAll(filepath.Dir(sess.Upper))

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

	// Unmount overlay
	if err := RemoveOverlay(sess.Merged); err != nil {
		return fmt.Errorf("unmount: %w", err)
	}

	// Delete upper and work dirs (discard all changes)
	os.RemoveAll(filepath.Dir(sess.Upper))

	// Update status
	sess.Status = StatusDiscarded
	return saveSession(m.sessionPath(id), sess)
}

// ListSessions returns all active sessions.
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

// syncUpperToLower copies changes from upper layer back to the lower directory.
func syncUpperToLower(upper, lower string) error {
	return filepath.WalkDir(upper, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(upper, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		target := filepath.Join(lower, rel)

		// Check for whiteout files (overlayfs marks deletions)
		if strings.HasPrefix(filepath.Base(rel), ".wh.") {
			original := filepath.Join(filepath.Dir(rel), strings.TrimPrefix(filepath.Base(rel), ".wh."))
			os.Remove(filepath.Join(lower, original))
			return nil
		}

		// Handle opaque directories (overlayfs marks directories where all
		// entries should be replaced)
		if d.IsDir() {
			// Check for opaque marker
			opaquePath := filepath.Join(path, ".wh..wh..opq")
			if _, err := os.Stat(opaquePath); err == nil {
				// Opaque: replace entire directory
				if err := os.RemoveAll(target); err != nil {
					return err
				}
				return copyDir(path, target)
			}

			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			return nil
		}

		// Regular file: copy from upper to lower
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

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		// Skip overlayfs opaque markers
		if e.Name() == ".wh..wh..opq" {
			continue
		}
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
