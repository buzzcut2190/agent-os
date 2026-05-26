package fs

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helpers

func setupSourceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0644)
	require.NoError(t, err)
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "inner.txt"), []byte("inner"), 0644))
	return dir
}

func newTestFS(t *testing.T) *FileSystem {
	t.Helper()
	dir := setupSourceDir(t)
	fs, err := NewFileSystem(dir)
	require.NoError(t, err)
	require.NotNil(t, fs)
	return fs
}

// parseDirentNames parses raw FUSE dirent bytes and returns entry names.
func parseDirentNames(data []byte) []string {
	const direntHeaderSize = 8 + 8 + 4 + 4 // ino + off + namelen + type
	const align = 8
	var names []string
	for len(data) >= direntHeaderSize {
		nameLen := binary.LittleEndian.Uint32(data[16:20])
		if direntHeaderSize+int(nameLen) > len(data) {
			break
		}
		name := string(data[direntHeaderSize : direntHeaderSize+nameLen])
		if name != "." && name != ".." {
			names = append(names, name)
		}
		entryLen := direntHeaderSize + int(nameLen)
		if pad := entryLen % align; pad != 0 {
			entryLen += align - pad
		}
		data = data[entryLen:]
	}
	return names
}

// readDirNames opens a directory, reads all entries, and returns sorted names.
func readDirNames(t *testing.T, fs *FileSystem, inode fuseops.InodeID) []string {
	t.Helper()
	ctx := context.Background()

	openOp := &fuseops.OpenDirOp{Inode: inode}
	require.NoError(t, fs.OpenDir(ctx, openOp))

	var names []string
	offset := fuseops.DirOffset(0)
	for {
		buf := make([]byte, 4096)
		readOp := &fuseops.ReadDirOp{
			Inode:  inode,
			Handle: openOp.Handle,
			Offset: offset,
			Dst:    buf,
		}
		require.NoError(t, fs.ReadDir(ctx, readOp))
		if readOp.BytesRead == 0 {
			break
		}
		entries := parseDirentNames(buf[:readOp.BytesRead])
		if len(entries) == 0 {
			break
		}
		names = append(names, entries...)
		// Advance offset past last entry (use BytesRead as approximation)
		offset += fuseops.DirOffset(readOp.BytesRead)
	}

	releaseOp := &fuseops.ReleaseDirHandleOp{Handle: openOp.Handle}
	_ = fs.ReleaseDirHandle(ctx, releaseOp)

	sort.Strings(names)
	return names
}

// Tests

func TestLookup(t *testing.T) {
	fs := newTestFS(t)
	ctx := context.Background()

	// Lookup existing file
	op := &fuseops.LookUpInodeOp{Parent: fuseops.RootInodeID, Name: "hello.txt"}
	err := fs.LookUpInode(ctx, op)
	assert.NoError(t, err, "lookup existing file should succeed")
	assert.NotEqual(t, fuseops.RootInodeID, op.Entry.Child, "should return a child inode")

	// Lookup existing directory
	op2 := &fuseops.LookUpInodeOp{Parent: fuseops.RootInodeID, Name: "subdir"}
	err = fs.LookUpInode(ctx, op2)
	assert.NoError(t, err, "lookup existing dir should succeed")

	// Lookup non-existent → ENOENT
	op3 := &fuseops.LookUpInodeOp{Parent: fuseops.RootInodeID, Name: "nonexistent.txt"}
	err = fs.LookUpInode(ctx, op3)
	assert.ErrorIs(t, err, fuse.ENOENT, "lookup non-existent should return ENOENT")
}

func TestGetAttr(t *testing.T) {
	fs := newTestFS(t)
	ctx := context.Background()

	// Root attributes
	op := &fuseops.GetInodeAttributesOp{Inode: fuseops.RootInodeID}
	err := fs.GetInodeAttributes(ctx, op)
	assert.NoError(t, err)
	assert.True(t, op.Attributes.Mode.IsDir(), "root should be a directory")

	// File attributes (lookup first to get inode)
	lookup := &fuseops.LookUpInodeOp{Parent: fuseops.RootInodeID, Name: "hello.txt"}
	require.NoError(t, fs.LookUpInode(ctx, lookup))

	attrOp := &fuseops.GetInodeAttributesOp{Inode: lookup.Entry.Child}
	err = fs.GetInodeAttributes(ctx, attrOp)
	assert.NoError(t, err)
	assert.Equal(t, uint64(len("hello world")), attrOp.Attributes.Size)
	assert.False(t, attrOp.Attributes.Mode.IsDir())

	since := time.Since(attrOp.Attributes.Mtime)
	assert.Less(t, since, time.Minute, "mtime should be recent")
}

func TestReadDir(t *testing.T) {
	fs := newTestFS(t)
	names := readDirNames(t, fs, fuseops.RootInodeID)
	assert.Equal(t, []string{"@bridges", "@complexity", "@context", "@daemon", "@dead-code", "@graph", "@kernel", "@lint", "@memory", "@providers", "@search", "@skills", "@tasks", "@team", "@workspaces", "hello.txt", "subdir"}, names)
}

func TestReadFile(t *testing.T) {
	fs := newTestFS(t)
	ctx := context.Background()
	content := "hello world"

	lookup := &fuseops.LookUpInodeOp{Parent: fuseops.RootInodeID, Name: "hello.txt"}
	require.NoError(t, fs.LookUpInode(ctx, lookup))

	openOp := &fuseops.OpenFileOp{Inode: lookup.Entry.Child}
	require.NoError(t, fs.OpenFile(ctx, openOp))

	dst := make([]byte, len(content)+16)
	readOp := &fuseops.ReadFileOp{
		Inode:  lookup.Entry.Child,
		Handle: openOp.Handle,
		Offset: 0,
		Size:   int64(len(content)),
		Dst:    dst,
	}
	require.NoError(t, fs.ReadFile(ctx, readOp))

	gotMD5 := md5.Sum(dst[:readOp.BytesRead])
	wantMD5 := md5.Sum([]byte(content))
	assert.Equal(t, fmt.Sprintf("%x", wantMD5), fmt.Sprintf("%x", gotMD5))

	_ = fs.ReleaseFileHandle(ctx, &fuseops.ReleaseFileHandleOp{Handle: openOp.Handle})
}

func TestWriteFile(t *testing.T) {
	fs := newTestFS(t)
	ctx := context.Background()

	createOp := &fuseops.CreateFileOp{
		Parent: fuseops.RootInodeID,
		Name:   "write_test.txt",
		Mode:   0644,
	}
	require.NoError(t, fs.CreateFile(ctx, createOp))

	newContent := []byte("written via fuse")
	writeOp := &fuseops.WriteFileOp{
		Inode:  createOp.Entry.Child,
		Handle: createOp.Handle,
		Offset: 0,
		Data:   newContent,
	}
	require.NoError(t, fs.WriteFile(ctx, writeOp))

	_ = fs.ReleaseFileHandle(ctx, &fuseops.ReleaseFileHandleOp{Handle: createOp.Handle})

	// Verify on source filesystem
	data, err := os.ReadFile(filepath.Join(fs.sourceDir, "write_test.txt"))
	require.NoError(t, err)
	gotMD5 := md5.Sum(data)
	wantMD5 := md5.Sum(newContent)
	assert.Equal(t, fmt.Sprintf("%x", wantMD5), fmt.Sprintf("%x", gotMD5))
}

func TestCreateFile(t *testing.T) {
	fs := newTestFS(t)
	ctx := context.Background()

	createOp := &fuseops.CreateFileOp{
		Parent: fuseops.RootInodeID,
		Name:   "created.txt",
		Mode:   0644,
	}
	err := fs.CreateFile(ctx, createOp)
	assert.NoError(t, err)

	_ = fs.ReleaseFileHandle(ctx, &fuseops.ReleaseFileHandleOp{Handle: createOp.Handle})

	_, err = os.Stat(filepath.Join(fs.sourceDir, "created.txt"))
	assert.NoError(t, err, "created file should exist in source dir")
}

func TestDeleteFile(t *testing.T) {
	fs := newTestFS(t)
	ctx := context.Background()

	op := &fuseops.UnlinkOp{Parent: fuseops.RootInodeID, Name: "hello.txt"}
	err := fs.Unlink(ctx, op)
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(fs.sourceDir, "hello.txt"))
	assert.True(t, os.IsNotExist(err), "file should be removed from source")
}

func TestMkdirRmdir(t *testing.T) {
	fs := newTestFS(t)
	ctx := context.Background()

	mkdirOp := &fuseops.MkDirOp{Parent: fuseops.RootInodeID, Name: "newdir"}
	err := fs.MkDir(ctx, mkdirOp)
	assert.NoError(t, err)

	stat, err := os.Stat(filepath.Join(fs.sourceDir, "newdir"))
	require.NoError(t, err)
	assert.True(t, stat.IsDir())

	rmdirOp := &fuseops.RmDirOp{Parent: fuseops.RootInodeID, Name: "newdir"}
	err = fs.RmDir(ctx, rmdirOp)
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(fs.sourceDir, "newdir"))
	assert.True(t, os.IsNotExist(err))
}

func TestRename(t *testing.T) {
	fs := newTestFS(t)
	ctx := context.Background()

	op := &fuseops.RenameOp{
		OldParent: fuseops.RootInodeID,
		NewParent: fuseops.RootInodeID,
		OldName:   "hello.txt",
		NewName:   "renamed.txt",
	}
	err := fs.Rename(ctx, op)
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(fs.sourceDir, "hello.txt"))
	assert.True(t, os.IsNotExist(err), "old name should not exist")

	data, err := os.ReadFile(filepath.Join(fs.sourceDir, "renamed.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestSymlinkReadlink(t *testing.T) {
	fs := newTestFS(t)
	ctx := context.Background()

	symlinkOp := &fuseops.CreateSymlinkOp{
		Parent: fuseops.RootInodeID,
		Name:   "link_to_hello",
		Target: "hello.txt",
	}
	err := fs.CreateSymlink(ctx, symlinkOp)
	assert.NoError(t, err)

	// Verify on source
	target, err := os.Readlink(filepath.Join(fs.sourceDir, "link_to_hello"))
	require.NoError(t, err)
	assert.Equal(t, "hello.txt", target)

	// Readlink via FUSE
	lookup := &fuseops.LookUpInodeOp{Parent: fuseops.RootInodeID, Name: "link_to_hello"}
	require.NoError(t, fs.LookUpInode(ctx, lookup))

	readlinkOp := &fuseops.ReadSymlinkOp{Inode: lookup.Entry.Child}
	err = fs.ReadSymlink(ctx, readlinkOp)
	assert.NoError(t, err)
	assert.Equal(t, "hello.txt", readlinkOp.Target)
}
