package fstest

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mountTimeout = 10 * time.Second

// buildAgentFS builds the agentfs binary and returns its path.
func buildAgentFS(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "agentfs")
	wd, err := os.Getwd()
	require.NoError(t, err)
	projectRoot := filepath.Join(wd, "../..")
	cmd := exec.Command("go", "build", "-ldflags", "-X main.version=0.1.0", "-o", bin, "./cmd/agentfs/")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(output))
	return bin
}

// waitForMount polls until the mount point is accessible.
func waitForMount(t *testing.T, mountPoint string) {
	t.Helper()
	deadline := time.Now().Add(mountTimeout)
	for time.Now().Before(deadline) {
		f, err := os.Open(mountPoint)
		if err == nil {
			f.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("mount point %s not ready within %v", mountPoint, mountTimeout)
}

// mountFUSE mounts the source dir at mountPoint, returns an unmount function.
func mountFUSE(t *testing.T, bin, sourceDir, mountPoint string) func() {
	t.Helper()

	require.NoError(t, os.MkdirAll(mountPoint, 0755))

	cmd := exec.Command(bin, "mount", sourceDir, mountPoint)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start(), "mount command failed to start")

	time.Sleep(500 * time.Millisecond)
	waitForMount(t, mountPoint)

	return func() {
		_ = exec.Command("fusermount", "-u", mountPoint).Run()
	}
}

// createTestFiles populates sourceDir with test files.
func createTestFiles(t *testing.T, sourceDir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "test.txt"), []byte("hello fuse"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "subdir", "nested.txt"), []byte("nested content"), 0644))
}

// requireFUSE skips if FUSE is not available.
func requireFUSE(t *testing.T) {
	t.Helper()
	f, err := os.OpenFile("/dev/fuse", os.O_RDONLY, 0)
	if err != nil {
		t.Skip("FUSE not available: " + err.Error())
	}
	f.Close()

	_, err = exec.LookPath("fusermount")
	if err != nil {
		t.Skip("fusermount not found")
	}
}

func TestPassThroughRead(t *testing.T) {
	requireFUSE(t)

	sourceDir := t.TempDir()
	mountPoint := filepath.Join(t.TempDir(), "mnt")
	createTestFiles(t, sourceDir)

	bin := buildAgentFS(t)
	unmount := mountFUSE(t, bin, sourceDir, mountPoint)
	defer unmount()

	data, err := os.ReadFile(filepath.Join(mountPoint, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello fuse", string(data))

	nested, err := os.ReadFile(filepath.Join(mountPoint, "subdir", "nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested content", string(nested))
}

func TestPassThroughWrite(t *testing.T) {
	requireFUSE(t)

	sourceDir := t.TempDir()
	mountPoint := filepath.Join(t.TempDir(), "mnt")
	createTestFiles(t, sourceDir)

	bin := buildAgentFS(t)
	unmount := mountFUSE(t, bin, sourceDir, mountPoint)
	defer unmount()

	newContent := []byte("written through fuse")
	err := os.WriteFile(filepath.Join(mountPoint, "test.txt"), newContent, 0644)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(sourceDir, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, newContent, data)
}

func TestPassThroughCreateDelete(t *testing.T) {
	requireFUSE(t)

	sourceDir := t.TempDir()
	mountPoint := filepath.Join(t.TempDir(), "mnt")
	createTestFiles(t, sourceDir)

	bin := buildAgentFS(t)
	unmount := mountFUSE(t, bin, sourceDir, mountPoint)
	defer unmount()

	fusePath := filepath.Join(mountPoint, "newfile.txt")
	require.NoError(t, os.WriteFile(fusePath, []byte("new"), 0644))

	_, err := os.Stat(filepath.Join(sourceDir, "newfile.txt"))
	assert.NoError(t, err, "file should exist in source after creation via FUSE")

	require.NoError(t, os.Remove(fusePath))

	_, err = os.Stat(filepath.Join(sourceDir, "newfile.txt"))
	assert.True(t, os.IsNotExist(err), "file should be removed from source")
}

func TestPassThroughAttr(t *testing.T) {
	requireFUSE(t)

	sourceDir := t.TempDir()
	mountPoint := filepath.Join(t.TempDir(), "mnt")
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "attr_test.txt"), []byte("attrs"), 0644))

	bin := buildAgentFS(t)
	unmount := mountFUSE(t, bin, sourceDir, mountPoint)
	defer unmount()

	fusePath := filepath.Join(mountPoint, "attr_test.txt")
	require.NoError(t, os.Chmod(fusePath, 0755))

	info, err := os.Stat(filepath.Join(sourceDir, "attr_test.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestLargeFile(t *testing.T) {
	requireFUSE(t)

	sourceDir := t.TempDir()
	mountPoint := filepath.Join(t.TempDir(), "mnt")

	size := 10 * 1024 * 1024
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "large.bin"), data, 0644))

	bin := buildAgentFS(t)
	unmount := mountFUSE(t, bin, sourceDir, mountPoint)
	defer unmount()

	f, err := os.Open(filepath.Join(mountPoint, "large.bin"))
	require.NoError(t, err)
	defer f.Close()

	h := md5.New()
	_, err = io.Copy(h, f)
	require.NoError(t, err)
	gotMD5 := fmt.Sprintf("%x", h.Sum(nil))

	wantMD5 := fmt.Sprintf("%x", md5.Sum(data))
	assert.Equal(t, wantMD5, gotMD5, "large file md5 should match")
}

func TestDeepNestedDir(t *testing.T) {
	requireFUSE(t)

	sourceDir := t.TempDir()
	mountPoint := filepath.Join(t.TempDir(), "mnt")

	deepPath := sourceDir
	for i := 0; i < 10; i++ {
		deepPath = filepath.Join(deepPath, fmt.Sprintf("level%d", i))
		require.NoError(t, os.MkdirAll(deepPath, 0755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(deepPath, "deep.txt"), []byte("deep"), 0644))

	bin := buildAgentFS(t)
	unmount := mountFUSE(t, bin, sourceDir, mountPoint)
	defer unmount()

	relPath := ""
	for i := 0; i < 10; i++ {
		relPath = filepath.Join(relPath, fmt.Sprintf("level%d", i))
	}
	data, err := os.ReadFile(filepath.Join(mountPoint, relPath, "deep.txt"))
	require.NoError(t, err)
	assert.Equal(t, "deep", string(data))
}

func TestConcurrentIO(t *testing.T) {
	requireFUSE(t)

	sourceDir := t.TempDir()
	mountPoint := filepath.Join(t.TempDir(), "mnt")

	for i := 0; i < 100; i++ {
		content := fmt.Sprintf("file %d content", i)
		require.NoError(t, os.WriteFile(filepath.Join(sourceDir, fmt.Sprintf("file_%03d.txt", i)), []byte(content), 0644))
	}

	bin := buildAgentFS(t)
	unmount := mountFUSE(t, bin, sourceDir, mountPoint)
	defer unmount()

	var wg sync.WaitGroup
	errCh := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("file_%03d.txt", idx)
			data, err := os.ReadFile(filepath.Join(mountPoint, name))
			if err != nil {
				errCh <- err
				return
			}
			expected := fmt.Sprintf("file %d content", idx)
			if string(data) != expected {
				errCh <- fmt.Errorf("%s: got %q, want %q", name, string(data), expected)
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent read error: %v", err)
	}
}

func TestSymlink(t *testing.T) {
	requireFUSE(t)

	sourceDir := t.TempDir()
	mountPoint := filepath.Join(t.TempDir(), "mnt")
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "target.txt"), []byte("symlink target"), 0644))
	require.NoError(t, os.Symlink("target.txt", filepath.Join(sourceDir, "link.txt")))

	bin := buildAgentFS(t)
	unmount := mountFUSE(t, bin, sourceDir, mountPoint)
	defer unmount()

	// Read symlink target
	target, err := os.Readlink(filepath.Join(mountPoint, "link.txt"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)

	// Read through symlink
	data, err := os.ReadFile(filepath.Join(mountPoint, "link.txt"))
	require.NoError(t, err)
	assert.Equal(t, "symlink target", string(data))
}

func TestMain(m *testing.M) {
	if _, err := os.Stat("/dev/fuse"); err != nil {
		fmt.Println("SKIP: /dev/fuse not available")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
