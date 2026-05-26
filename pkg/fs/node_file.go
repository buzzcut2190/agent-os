package fs

import (
	"context"
	"os"
	"path/filepath"
	"syscall"

	"github.com/jacobsa/fuse/fuseops"
)

func (fs *FileSystem) CreateFile(_ context.Context, op *fuseops.CreateFileOp) error {
	if op.Parent == fuseops.RootInodeID && isVirtualFileName(op.Name) {
		return syscall.EPERM
	}

	parentPath, err := fs.pathForInode(op.Parent)
	if err != nil {
		return err
	}
	filePath := filepath.Join(parentPath, op.Name)

	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, op.Mode)
	if err != nil {
		return err
	}

	ino, err := fs.registerPath(filePath)
	if err != nil {
		f.Close()
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}

	handle := fs.allocHandle()
	fs.handleMu.Lock()
	fs.files[handle] = f
	fs.handleMu.Unlock()

	op.Entry = fuseops.ChildInodeEntry{
		Child:                ino,
		Generation:           1,
		Attributes:           statToAttributes(info),
		AttributesExpiration: oneSecond(),
		EntryExpiration:      oneSecond(),
	}
	op.Handle = handle
	return nil
}

func (fs *FileSystem) OpenFile(_ context.Context, op *fuseops.OpenFileOp) error {
	// Virtual files; assign a dummy handle.
	if fs.isContextInode(op.Inode) || fs.isRefactorInode(op.Inode) {
		handle := fs.allocHandle()
		fs.handleMu.Lock()
		fs.files[handle] = nil // nil file signals virtual
		fs.handleMu.Unlock()
		op.Handle = handle
		op.KeepPageCache = true
		return nil
	}
	// Virtual @search and @graph files.
	if fs.isSearchInode(op.Inode) || fs.isGraphInode(op.Inode) {
		handle := fs.allocHandle()
		fs.handleMu.Lock()
		fs.files[handle] = nil
		fs.handleMu.Unlock()
		op.Handle = handle
		op.KeepPageCache = true
		return nil
	}
	// Virtual @team and @tasks files.
	if fs.isTeamInode(op.Inode) || fs.isTasksInode(op.Inode) {
		handle := fs.allocHandle()
		fs.handleMu.Lock()
		fs.files[handle] = nil
		fs.handleMu.Unlock()
		op.Handle = handle
		op.KeepPageCache = true
		return nil
	}
	// Virtual @skills files.
	if fs.isSkillsInode(op.Inode) {
		handle := fs.allocHandle()
		fs.handleMu.Lock()
		fs.files[handle] = nil
		fs.handleMu.Unlock()
		op.Handle = handle
		op.KeepPageCache = true
		return nil
	}
	// Virtual @providers files.
	if fs.isProvidersInode(op.Inode) {
		handle := fs.allocHandle()
		fs.handleMu.Lock()
		fs.files[handle] = nil
		fs.handleMu.Unlock()
		op.Handle = handle
		op.KeepPageCache = true
		return nil
	}
	// Virtual @memory files.
	if fs.isMemoryInode(op.Inode) {
		handle := fs.allocHandle()
		fs.handleMu.Lock()
		fs.files[handle] = nil
		fs.handleMu.Unlock()
		op.Handle = handle
		op.KeepPageCache = true
		return nil
	}
	// Virtual @bridges files.
	if fs.isBridgesInode(op.Inode) {
		handle := fs.allocHandle()
		fs.handleMu.Lock()
		fs.files[handle] = nil
		fs.handleMu.Unlock()
		op.Handle = handle
		op.KeepPageCache = true
		return nil
	}
	// Virtual @kernel files.
	if fs.isKernelInode(op.Inode) {
		handle := fs.allocHandle()
		fs.handleMu.Lock()
		fs.files[handle] = nil
		fs.handleMu.Unlock()
		op.Handle = handle
		op.KeepPageCache = true
		return nil
	}
	// Virtual @daemon files.
	if fs.isDaemonInode(op.Inode) {
		handle := fs.allocHandle()
		fs.handleMu.Lock()
		fs.files[handle] = nil
		fs.handleMu.Unlock()
		op.Handle = handle
		op.KeepPageCache = true
		return nil
	}
	// Virtual @workspaces files.
	if fs.isWorkspacesInode(op.Inode) {
		handle := fs.allocHandle()
		fs.handleMu.Lock()
		fs.files[handle] = nil
		fs.handleMu.Unlock()
		op.Handle = handle
		op.KeepPageCache = true
		return nil
	}

	path, err := fs.pathForInode(op.Inode)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		f, err = os.Open(path)
		if err != nil {
			return err
		}
	}

	handle := fs.allocHandle()
	fs.handleMu.Lock()
	fs.files[handle] = f
	fs.handleMu.Unlock()

	op.Handle = handle
	op.KeepPageCache = true
	return nil
}

func (fs *FileSystem) ReadFile(_ context.Context, op *fuseops.ReadFileOp) error {
	if fs.tryContextRead(op) {
		return nil
	}
	if fs.tryRefactorRead(op) {
		return nil
	}
	if fs.trySearchRead(op) {
		return nil
	}
	if fs.tryGraphRead(op) {
		return nil
	}
	if fs.tryTeamRead(op) {
		return nil
	}
	if fs.tryTasksRead(op) {
		return nil
	}
	if fs.trySkillsRead(op) {
		return nil
	}
	if fs.tryProvidersRead(op) {
		return nil
	}
	if fs.tryMemoryRead(op) {
		return nil
	}
	if fs.tryBridgesRead(op) {
		return nil
	}
	if fs.tryKernelRead(op) {
		return nil
	}
	if fs.tryDaemonRead(op) {
		return nil
	}
	if fs.tryWorkspacesRead(op) {
		return nil
	}

	fs.handleMu.Lock()
	f, ok := fs.files[op.Handle]
	fs.handleMu.Unlock()
	if !ok || f == nil {
		return syscall.EBADF
	}

	n, err := f.ReadAt(op.Dst, op.Offset)
	if err != nil && n == 0 {
		return err
	}
	op.BytesRead = n
	return nil
}

func (fs *FileSystem) WriteFile(_ context.Context, op *fuseops.WriteFileOp) error {
	if fs.isContextInode(op.Inode) || fs.isRefactorInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.isSearchInode(op.Inode) || fs.isGraphInode(op.Inode) {
		return syscall.EPERM
	}
	// @team and @tasks: some inodes are writable.
	if fs.tryTeamWrite(op) {
		return nil
	}
	if fs.tryTasksWrite(op) {
		return nil
	}
	if fs.isTeamInode(op.Inode) || fs.isTasksInode(op.Inode) {
		return syscall.EPERM
	}
	// @skills: state file is writable.
	if fs.trySkillsWrite(op) {
		return nil
	}
	if fs.isSkillsInode(op.Inode) {
		return syscall.EPERM
	}
	// @providers: some inodes are writable.
	if fs.tryProvidersWrite(op) {
		return nil
	}
	if fs.isProvidersInode(op.Inode) {
		return syscall.EPERM
	}
	// @memory and @bridges: some inodes are writable.
	if fs.tryMemoryWrite(op) {
		return nil
	}
	if fs.isMemoryInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.tryBridgesWrite(op) {
		return nil
	}
	if fs.isBridgesInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.tryKernelWrite(op) {
		return nil
	}
	if fs.isKernelInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.tryDaemonWrite(op) {
		return nil
	}
	if fs.isDaemonInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.tryWorkspacesWrite(op) {
		return nil
	}
	if fs.isWorkspacesInode(op.Inode) {
		return syscall.EPERM
	}

	fs.handleMu.Lock()
	f, ok := fs.files[op.Handle]
	fs.handleMu.Unlock()
	if !ok || f == nil {
		return syscall.EBADF
	}

	_, err := f.WriteAt(op.Data, op.Offset)
	return err
}

func (fs *FileSystem) FlushFile(_ context.Context, op *fuseops.FlushFileOp) error {
	fs.handleMu.Lock()
	f, ok := fs.files[op.Handle]
	fs.handleMu.Unlock()
	if !ok {
		return syscall.EBADF
	}
	if f == nil {
		return nil // virtual file handle
	}
	return f.Sync()
}

func (fs *FileSystem) ReleaseFileHandle(_ context.Context, op *fuseops.ReleaseFileHandleOp) error {
	fs.handleMu.Lock()
	f, ok := fs.files[op.Handle]
	if ok {
		delete(fs.files, op.Handle)
	}
	fs.handleMu.Unlock()
	if !ok {
		return syscall.EBADF
	}
	if f == nil {
		return nil // virtual @context handle
	}
	return f.Close()
}

func (fs *FileSystem) CreateSymlink(_ context.Context, op *fuseops.CreateSymlinkOp) error {
	if op.Parent == fuseops.RootInodeID && isVirtualFileName(op.Name) {
		return syscall.EPERM
	}

	parentPath, err := fs.pathForInode(op.Parent)
	if err != nil {
		return err
	}
	linkPath := filepath.Join(parentPath, op.Name)

	if err := os.Symlink(op.Target, linkPath); err != nil {
		return err
	}
	ino, err := fs.registerPath(linkPath)
	if err != nil {
		return err
	}
	info, err := os.Lstat(linkPath)
	if err != nil {
		return err
	}
	op.Entry = fuseops.ChildInodeEntry{
		Child:                ino,
		Generation:           1,
		Attributes:           statToAttributes(info),
		AttributesExpiration: oneSecond(),
		EntryExpiration:      oneSecond(),
	}
	return nil
}

func (fs *FileSystem) ReadSymlink(_ context.Context, op *fuseops.ReadSymlinkOp) error {
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
		return syscall.EPERM
	}
	if fs.isProvidersInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.isMemoryInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.isBridgesInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.isKernelInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.isDaemonInode(op.Inode) {
		return syscall.EPERM
	}
	if fs.isWorkspacesInode(op.Inode) {
		return syscall.EPERM
	}

	path, err := fs.pathForInode(op.Inode)
	if err != nil {
		return err
	}
	target, err := os.Readlink(path)
	if err != nil {
		return err
	}
	op.Target = target
	return nil
}

func (fs *FileSystem) CreateLink(_ context.Context, op *fuseops.CreateLinkOp) error {
	if op.Parent == fuseops.RootInodeID && isVirtualFileName(op.Name) {
		return syscall.EPERM
	}

	parentPath, err := fs.pathForInode(op.Parent)
	if err != nil {
		return err
	}
	targetPath, err := fs.pathForInode(op.Target)
	if err != nil {
		return err
	}
	linkPath := filepath.Join(parentPath, op.Name)

	if err := os.Link(targetPath, linkPath); err != nil {
		return err
	}
	ino, err := fs.registerPath(linkPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(linkPath)
	if err != nil {
		return err
	}
	op.Entry = fuseops.ChildInodeEntry{
		Child:                ino,
		Generation:           1,
		Attributes:           statToAttributes(info),
		AttributesExpiration: oneSecond(),
		EntryExpiration:      oneSecond(),
	}
	return nil
}
