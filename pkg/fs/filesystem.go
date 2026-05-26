package fs

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
)

// NewFileSystem creates a new passthrough FUSE filesystem rooted at sourceDir.
func NewFileSystem(sourceDir string) (*FileSystem, error) {
	abs, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fuse.ENOSYS
	}

	fs := &FileSystem{
		sourceDir:  abs,
		inodeMap:   make(map[fuseops.InodeID]string),
		rootInode:  fuseops.InodeID(stat.Ino),
		nextHandle: 1,
		files:      make(map[fuseops.HandleID]*os.File),
		dirs:       make(map[fuseops.HandleID]*dirHandle),
	}
	fs.inodeMap[fuseops.RootInodeID] = abs
	fs.inodeMap[fs.rootInode] = abs
	return fs, nil
}

func (fs *FileSystem) pathForInode(id fuseops.InodeID) (string, error) {
	if id == fuseops.RootInodeID {
		return fs.sourceDir, nil
	}
	fs.mu.RLock()
	p, ok := fs.inodeMap[id]
	fs.mu.RUnlock()
	if !ok {
		return "", fuse.ENOENT
	}
	return p, nil
}

func (fs *FileSystem) registerPath(absPath string) (fuseops.InodeID, error) {
	info, err := os.Lstat(absPath)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fuse.ENOSYS
	}
	ino := fuseops.InodeID(stat.Ino)

	if absPath == fs.sourceDir {
		fs.mu.Lock()
		fs.inodeMap[fuseops.RootInodeID] = absPath
		fs.inodeMap[ino] = absPath
		fs.mu.Unlock()
		return ino, nil
	}

	fs.mu.Lock()
	fs.inodeMap[ino] = absPath
	fs.mu.Unlock()
	return ino, nil
}

func (fs *FileSystem) allocHandle() fuseops.HandleID {
	fs.handleMu.Lock()
	defer fs.handleMu.Unlock()
	h := fs.nextHandle
	fs.nextHandle++
	return h
}

func statToAttributes(info os.FileInfo) fuseops.InodeAttributes {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fuseops.InodeAttributes{Size: uint64(info.Size()), Mode: info.Mode()}
	}
	return fuseops.InodeAttributes{
		Size:  uint64(info.Size()),
		Nlink: uint32(stat.Nlink),
		Mode:  info.Mode(),
		Rdev:  uint32(stat.Rdev),
		Atime: time.Unix(int64(stat.Atim.Sec), int64(stat.Atim.Nsec)),
		Mtime: time.Unix(int64(stat.Mtim.Sec), int64(stat.Mtim.Nsec)),
		Ctime: time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec)),
		Uid:   uint32(stat.Uid),
		Gid:   uint32(stat.Gid),
	}
}

func oneSecond() time.Time { return time.Now().Add(time.Second) }

func (fs *FileSystem) StatFS(_ context.Context, op *fuseops.StatFSOp) error {
	var s syscall.Statfs_t
	if err := syscall.Statfs(fs.sourceDir, &s); err != nil {
		return err
	}
	op.BlockSize = uint32(s.Frsize)
	op.Blocks = s.Blocks
	op.BlocksFree = s.Bfree
	op.BlocksAvailable = s.Bavail
	op.IoSize = uint32(s.Bsize)
	op.Inodes = s.Files
	op.InodesFree = s.Ffree
	return nil
}

func (fs *FileSystem) LookUpInode(_ context.Context, op *fuseops.LookUpInodeOp) error {
	// Check for virtual @context file
	if op.Parent == fuseops.RootInodeID {
		if entry, ok := fs.tryContextLookup(op.Name); ok {
			op.Entry = *entry
			return nil
		}
		if entry, ok := fs.tryRefactorLookup(op.Name); ok {
			op.Entry = *entry
			return nil
		}
	}

	// Check for virtual @search entries
	if entry, ok := fs.trySearchLookup(op.Name, op.Parent); ok {
		op.Entry = *entry
		return nil
	}
	// Check for virtual @graph entries
	if entry, ok := fs.tryGraphLookup(op.Name, op.Parent); ok {
		op.Entry = *entry
		return nil
	}
	// Check for virtual @team entries
	if entry, ok := fs.tryTeamLookup(op.Name, op.Parent); ok {
		op.Entry = *entry
		return nil
	}
	// Check for virtual @tasks entries
	if entry, ok := fs.tryTasksLookup(op.Name, op.Parent); ok {
		op.Entry = *entry
		return nil
	}
	// Check for virtual @skills entries
	if entry, ok := fs.trySkillsLookup(op.Name, op.Parent); ok {
		op.Entry = *entry
		return nil
	}
	// Check for virtual @providers entries
	if entry, ok := fs.tryProvidersLookup(op.Name, op.Parent); ok {
		op.Entry = *entry
		return nil
	}
	// Check for virtual @memory entries
	if entry, ok := fs.tryMemoryLookup(op.Name, op.Parent); ok {
		op.Entry = *entry
		return nil
	}
	// Check for virtual @bridges entries
	if entry, ok := fs.tryBridgesLookup(op.Name, op.Parent); ok {
		op.Entry = *entry
		return nil
	}
	// Check for virtual @kernel entries
	if entry, ok := fs.tryKernelLookup(op.Name, op.Parent); ok {
		op.Entry = *entry
		return nil
	}
	// Check for virtual @daemon entries
	if entry, ok := fs.tryDaemonLookup(op.Name, op.Parent); ok {
		op.Entry = *entry
		return nil
	}
	// Check for virtual @workspaces entries
	if entry, ok := fs.tryWorkspacesLookup(op.Name, op.Parent); ok {
		op.Entry = *entry
		return nil
	}

	parentPath, err := fs.pathForInode(op.Parent)
	if err != nil {
		return err
	}
	childPath := filepath.Join(parentPath, op.Name)

	info, err := os.Lstat(childPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fuse.ENOENT
		}
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fuse.ENOSYS
	}
	ino := fuseops.InodeID(stat.Ino)

	fs.mu.Lock()
	fs.inodeMap[ino] = childPath
	fs.mu.Unlock()

	op.Entry = fuseops.ChildInodeEntry{
		Child:                ino,
		Generation:           1,
		Attributes:           statToAttributes(info),
		AttributesExpiration: oneSecond(),
		EntryExpiration:      oneSecond(),
	}
	return nil
}

func (fs *FileSystem) GetInodeAttributes(_ context.Context, op *fuseops.GetInodeAttributesOp) error {
	if fs.tryContextGetAttr(op) {
		return nil
	}
	if fs.tryRefactorGetAttr(op) {
		return nil
	}
	if fs.trySearchGetAttr(op) {
		return nil
	}
	if fs.tryGraphGetAttr(op) {
		return nil
	}
	if fs.tryTeamGetAttr(op) {
		return nil
	}
	if fs.tryTasksGetAttr(op) {
		return nil
	}
	if fs.trySkillsGetAttr(op) {
		return nil
	}
	if fs.tryProvidersGetAttr(op) {
		return nil
	}
	if fs.tryMemoryGetAttr(op) {
		return nil
	}
	if fs.tryBridgesGetAttr(op) {
		return nil
	}
	if fs.tryKernelGetAttr(op) {
		return nil
	}
	if fs.tryDaemonGetAttr(op) {
		return nil
	}
	if fs.tryWorkspacesGetAttr(op) {
		return nil
	}

	path, err := fs.pathForInode(op.Inode)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fuse.ENOENT
	}
	op.Attributes = statToAttributes(info)
	op.AttributesExpiration = oneSecond()
	return nil
}

func (fs *FileSystem) SetInodeAttributes(_ context.Context, op *fuseops.SetInodeAttributesOp) error {
	if fs.isContextInode(op.Inode) || fs.isRefactorInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.isSearchInode(op.Inode) || fs.isGraphInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.isTeamInode(op.Inode) || fs.isTasksInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.isSkillsInode(op.Inode) {
		if fs.isSkillsStateWritable(op.Inode) {
			return nil
		}
		return syscall.EPERM
	}
	if fs.isProvidersInode(op.Inode) {
		if fs.isProvidersStateWritable(op.Inode) {
			return nil
		}
		return syscall.EPERM
	}
	if fs.isMemoryInode(op.Inode) {
		if fs.isMemoryStateWritable(op.Inode) {
			return nil
		}
		return syscall.EPERM
	}
	if fs.isBridgesInode(op.Inode) {
		if fs.isBridgesStateWritable(op.Inode) {
			return nil
		}
		return syscall.EPERM
	}
	if fs.isKernelInode(op.Inode) {
		if fs.isKernelStateWritable(op.Inode) {
			return nil
		}
		return syscall.EPERM
	}
	if fs.isDaemonInode(op.Inode) {
		if fs.isDaemonStateWritable(op.Inode) {
			return nil
		}
		return syscall.EPERM
	}
	if fs.isWorkspacesInode(op.Inode) {
		if fs.isWorkspacesStateWritable(op.Inode) {
			return nil
		}
		return syscall.EPERM
	}

	path, err := fs.pathForInode(op.Inode)
	if err != nil {
		return err
	}

	if op.Mode != nil {
		if err := os.Chmod(path, *op.Mode); err != nil {
			return err
		}
	}
	if op.Size != nil {
		if err := os.Truncate(path, int64(*op.Size)); err != nil {
			return err
		}
	}
	if op.Atime != nil && op.Mtime != nil {
		if err := os.Chtimes(path, *op.Atime, *op.Mtime); err != nil {
			return err
		}
	} else if op.Atime != nil {
		if err := os.Chtimes(path, *op.Atime, time.Time{}); err != nil {
			return err
		}
	} else if op.Mtime != nil {
		if err := os.Chtimes(path, time.Time{}, *op.Mtime); err != nil {
			return err
		}
	}

	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	op.Attributes = statToAttributes(info)
	op.AttributesExpiration = oneSecond()
	return nil
}

func (fs *FileSystem) ForgetInode(_ context.Context, op *fuseops.ForgetInodeOp) error {
	if op.Inode == fuseops.RootInodeID || op.Inode == fs.rootInode {
		return nil
	}
	if fs.isContextInode(op.Inode) || fs.isRefactorInode(op.Inode) {
		return nil
	}
	if fs.isSearchInode(op.Inode) || fs.isGraphInode(op.Inode) {
		return nil
	}
	if fs.isTeamInode(op.Inode) || fs.isTasksInode(op.Inode) {
		return nil
	}
	if fs.isSkillsInode(op.Inode) {
		return nil
	}
	if fs.isProvidersInode(op.Inode) {
		return nil
	}
	if fs.isMemoryInode(op.Inode) {
		return nil
	}
	if fs.isBridgesInode(op.Inode) {
		return nil
	}
	if fs.isKernelInode(op.Inode) {
		return nil
	}
	if fs.isDaemonInode(op.Inode) {
		return nil
	}
	if fs.isWorkspacesInode(op.Inode) {
		return nil
	}
	fs.mu.Lock()
	delete(fs.inodeMap, op.Inode)
	fs.mu.Unlock()
	return nil
}

