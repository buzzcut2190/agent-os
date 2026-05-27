package sandbox

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hello"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0644))
	return dir
}

func computeDirHash(t *testing.T, dir string) string {
	t.Helper()
	var hashes []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		hashes = append(hashes, fmt.Sprintf("%x", md5.Sum(data)))
		return nil
	})
	h := md5.New()
	for _, s := range hashes {
		h.Write([]byte(s))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func TestCreateWorkspace(t *testing.T) {
	project := setupTestProject(t)
	workspace := filepath.Join(t.TempDir(), "workspace")

	err := CreateWorkspace(project, workspace)
	require.NoError(t, err)

	// Should see project files in workspace
	data, err := os.ReadFile(filepath.Join(workspace, "readme.md"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	data, err = os.ReadFile(filepath.Join(workspace, "src", "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main", string(data))
}

func TestSessionStart(t *testing.T) {
	project := setupTestProject(t)
	mgr := NewManager(t.TempDir())

	sess, err := mgr.StartSession(project)
	require.NoError(t, err)
	assert.Equal(t, StatusActive, sess.Status)
	assert.Equal(t, project, sess.Project)
	assert.NotEmpty(t, sess.Workspace)
	assert.DirExists(t, sess.Workspace)

	// Workspace should contain project files
	_, err = os.Stat(filepath.Join(sess.Workspace, "readme.md"))
	assert.NoError(t, err)
}

func TestSessionCommit(t *testing.T) {
	project := setupTestProject(t)
	originalHash := computeDirHash(t, project)

	mgr := NewManager(t.TempDir())
	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	// Modify in workspace
	require.NoError(t, os.WriteFile(filepath.Join(sess.Workspace, "newfile.go"), []byte("package main"), 0644))

	// Commit
	require.NoError(t, mgr.CommitSession(sess.ID))

	// Project should have the new file
	_, err = os.Stat(filepath.Join(project, "newfile.go"))
	assert.NoError(t, err, "committed file should exist in project")
	assert.NotEqual(t, originalHash, computeDirHash(t, project), "project hash should change after commit")

	// Workspace should be cleaned up
	assert.NoDirExists(t, sess.Workspace)

	// Load the session — should be in committed state
	loaded, err := mgr.GetSession(sess.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCommitted, loaded.Status)
}

func TestSessionCommitModify(t *testing.T) {
	project := setupTestProject(t)

	mgr := NewManager(t.TempDir())
	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	// Modify existing file in workspace
	require.NoError(t, os.WriteFile(filepath.Join(sess.Workspace, "readme.md"), []byte("modified"), 0644))

	// Commit
	require.NoError(t, mgr.CommitSession(sess.ID))

	// Project file should be modified
	data, err := os.ReadFile(filepath.Join(project, "readme.md"))
	require.NoError(t, err)
	assert.Equal(t, "modified", string(data))
}

func TestSessionCommitDelete(t *testing.T) {
	project := setupTestProject(t)

	mgr := NewManager(t.TempDir())
	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	// Delete a file in workspace
	require.NoError(t, os.Remove(filepath.Join(sess.Workspace, "readme.md")))

	// Commit
	require.NoError(t, mgr.CommitSession(sess.ID))

	// File should be deleted from project
	_, err = os.Stat(filepath.Join(project, "readme.md"))
	assert.True(t, os.IsNotExist(err))
}

func TestSessionDiscard(t *testing.T) {
	project := setupTestProject(t)
	originalHash := computeDirHash(t, project)

	mgr := NewManager(t.TempDir())
	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	// Modify in workspace
	require.NoError(t, os.WriteFile(filepath.Join(sess.Workspace, "temp.go"), []byte("temp"), 0644))

	// Discard
	require.NoError(t, mgr.DiscardSession(sess.ID))

	// Project should be unchanged
	assert.Equal(t, originalHash, computeDirHash(t, project), "project should be unchanged after discard")
	_, err = os.Stat(filepath.Join(project, "temp.go"))
	assert.True(t, os.IsNotExist(err))

	// Workspace should be cleaned up
	assert.NoDirExists(t, sess.Workspace)

	// Session should be in discarded state
	loaded, err := mgr.GetSession(sess.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusDiscarded, loaded.Status)
}

func TestSessionList(t *testing.T) {
	project := setupTestProject(t)
	mgr := NewManager(t.TempDir())

	sess1, err := mgr.StartSession(project)
	require.NoError(t, err)
	defer func() { _ = mgr.DiscardSession(sess1.ID) }()

	sess2, err := mgr.StartSession(project)
	require.NoError(t, err)
	defer func() { _ = mgr.DiscardSession(sess2.ID) }()

	sessions, err := mgr.ListSessions()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(sessions), 2)
}

func TestConcurrentSessions(t *testing.T) {
	project := setupTestProject(t)
	mgr := NewManager(t.TempDir())
	var wg sync.WaitGroup
	var ids []string
	var mu sync.Mutex

	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sess, err := mgr.StartSession(project)
			if err != nil {
				t.Errorf("start session %d: %v", idx, err)
				return
			}
			mu.Lock()
			ids = append(ids, sess.ID)
			mu.Unlock()
			// Write a unique file
			_ = os.WriteFile(filepath.Join(sess.Workspace, fmt.Sprintf("from_session_%d.txt", idx)),
				[]byte("data"), 0644)
		}(i)
	}
	wg.Wait()
	assert.Len(t, ids, 15, "should create 15 sessions")

	// Discard all
	for _, id := range ids {
		require.NoError(t, mgr.DiscardSession(id))
	}
}

func TestDiffSessionNoChanges(t *testing.T) {
	project := setupTestProject(t)
	mgr := NewManager(t.TempDir())

	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	changes, err := mgr.DiffSession(sess.ID)
	require.NoError(t, err)
	assert.Empty(t, changes, "no changes should be detected for a fresh session")
}

func TestDiffSessionAdded(t *testing.T) {
	project := setupTestProject(t)
	mgr := NewManager(t.TempDir())

	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	// Add a new file
	require.NoError(t, os.WriteFile(filepath.Join(sess.Workspace, "newfile.txt"), []byte("new content"), 0644))

	changes, err := mgr.DiffSession(sess.ID)
	require.NoError(t, err)
	assert.Len(t, changes, 1)
	assert.Equal(t, "newfile.txt", changes[0].Path)
	assert.Equal(t, "added", changes[0].Status)
}

func TestDiffSessionModified(t *testing.T) {
	project := setupTestProject(t)
	mgr := NewManager(t.TempDir())

	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	// Modify an existing file
	require.NoError(t, os.WriteFile(filepath.Join(sess.Workspace, "readme.md"), []byte("modified content"), 0644))

	changes, err := mgr.DiffSession(sess.ID)
	require.NoError(t, err)
	assert.Len(t, changes, 1)
	assert.Equal(t, "readme.md", changes[0].Path)
	assert.Equal(t, "modified", changes[0].Status)
}

func TestDiffSessionDeleted(t *testing.T) {
	project := setupTestProject(t)
	mgr := NewManager(t.TempDir())

	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	// Delete a file
	require.NoError(t, os.Remove(filepath.Join(sess.Workspace, "readme.md")))

	changes, err := mgr.DiffSession(sess.ID)
	require.NoError(t, err)
	assert.Len(t, changes, 1)
	assert.Equal(t, "readme.md", changes[0].Path)
	assert.Equal(t, "deleted", changes[0].Status)
}

func TestDiffSessionMixed(t *testing.T) {
	project := setupTestProject(t)
	mgr := NewManager(t.TempDir())

	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	// Add a new file
	require.NoError(t, os.WriteFile(filepath.Join(sess.Workspace, "added.txt"), []byte("new"), 0644))
	// Modify an existing file
	require.NoError(t, os.WriteFile(filepath.Join(sess.Workspace, "readme.md"), []byte("changed"), 0644))
	// Delete an existing file
	require.NoError(t, os.Remove(filepath.Join(sess.Workspace, filepath.Join("src", "main.go"))))

	changes, err := mgr.DiffSession(sess.ID)
	require.NoError(t, err)

	// Build a map for easy assertions
	byPath := make(map[string]string)
	for _, c := range changes {
		byPath[c.Path] = c.Status
	}
	assert.Equal(t, "added", byPath["added.txt"])
	assert.Equal(t, "modified", byPath["readme.md"])
	assert.Equal(t, "deleted", byPath[filepath.Join("src", "main.go")])
	assert.Len(t, changes, 3)
}

func TestDiffSessionNestedAdded(t *testing.T) {
	project := setupTestProject(t)
	mgr := NewManager(t.TempDir())

	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	// Add a file in a new subdirectory
	require.NoError(t, os.MkdirAll(filepath.Join(sess.Workspace, "sub", "dir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sess.Workspace, "sub", "dir", "nested.go"), []byte("pkg nested"), 0644))

	changes, err := mgr.DiffSession(sess.ID)
	require.NoError(t, err)
	assert.Len(t, changes, 1)
	assert.Equal(t, filepath.Join("sub", "dir", "nested.go"), changes[0].Path)
	assert.Equal(t, "added", changes[0].Status)
}
