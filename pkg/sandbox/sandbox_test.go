package sandbox

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireOverlayFS(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/sys/module/overlay"); err != nil {
		t.Skip("overlay kernel module not available")
	}
	// Test if we can actually mount (requires privileges)
	merged := t.TempDir()
	err := createOverlay(t.TempDir(), t.TempDir(), t.TempDir(), merged)
	if err != nil {
		t.Skipf("overlay mount not permitted: %v", err)
	}
	_ = syscall.Unmount(merged, 0)
}

func createOverlay(lower, upper, work, merged string) error {
	for _, dir := range []string{upper, work, merged} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return syscall.Mount("overlay", merged, "overlay", 0,
		fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lower, upper, work))
}

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

func TestCreateOverlay(t *testing.T) {
	requireOverlayFS(t)

	project := setupTestProject(t)
	lower := project
	upper := filepath.Join(t.TempDir(), "upper")
	work := filepath.Join(t.TempDir(), "work")
	merged := filepath.Join(t.TempDir(), "merged")

	err := CreateOverlay(lower, upper, work, merged)
	require.NoError(t, err)
	defer func() { _ = RemoveOverlay(merged) }()

	// Should see project files
	data, err := os.ReadFile(filepath.Join(merged, "readme.md"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	// Write through overlay should appear in upper
	require.NoError(t, os.WriteFile(filepath.Join(merged, "new.txt"), []byte("created"), 0644))
	_, err = os.Stat(filepath.Join(upper, "new.txt"))
	assert.NoError(t, err, "new file should appear in upper layer")
}

func TestSessionCommit(t *testing.T) {
	requireOverlayFS(t)

	project := setupTestProject(t)
	originalHash := computeDirHash(t, project)

	mgr := NewManager(t.TempDir())
	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	// Modify via overlay
	require.NoError(t, os.WriteFile(filepath.Join(sess.Merged, "newfile.go"), []byte("package main"), 0644))

	// Commit
	require.NoError(t, mgr.CommitSession(sess.ID))

	// Project should have the new file
	_, err = os.Stat(filepath.Join(project, "newfile.go"))
	assert.NoError(t, err, "committed file should exist in project")
	assert.NotEqual(t, originalHash, computeDirHash(t, project), "project hash should change after commit")

	// Load the session - should be in committed state
	loaded, err := mgr.GetSession(sess.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCommitted, loaded.Status)
}

func TestSessionDiscard(t *testing.T) {
	requireOverlayFS(t)

	project := setupTestProject(t)
	originalHash := computeDirHash(t, project)

	mgr := NewManager(t.TempDir())
	sess, err := mgr.StartSession(project)
	require.NoError(t, err)

	// Modify via overlay
	require.NoError(t, os.WriteFile(filepath.Join(sess.Merged, "temp.go"), []byte("temp"), 0644))

	// Discard
	require.NoError(t, mgr.DiscardSession(sess.ID))

	// Project should be unchanged
	assert.Equal(t, originalHash, computeDirHash(t, project), "project should be unchanged after discard")
	_, err = os.Stat(filepath.Join(project, "temp.go"))
	assert.True(t, os.IsNotExist(err))

	// Session should be in discarded state
	loaded, err := mgr.GetSession(sess.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusDiscarded, loaded.Status)
}

func TestSessionList(t *testing.T) {
	requireOverlayFS(t)

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
	requireOverlayFS(t)

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
			_ = os.WriteFile(filepath.Join(sess.Merged, fmt.Sprintf("from_session_%d.txt", idx)),
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
