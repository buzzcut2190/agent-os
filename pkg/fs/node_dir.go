package fs

import (
	"context"
	"os"
	"path/filepath"
	"syscall"

	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

func (fs *FileSystem) OpenDir(_ context.Context, op *fuseops.OpenDirOp) error {
	// Handle virtual @search directories
	if fs.trySearchOpenDir(op) {
		return nil
	}
	// Handle virtual @graph directory
	if fs.tryGraphOpenDir(op) {
		return nil
	}
	// Handle virtual @team directories
	if fs.tryTeamOpenDir(op) {
		return nil
	}
	// Handle virtual @tasks directories
	if fs.tryTasksOpenDir(op) {
		return nil
	}
	// Handle virtual @skills directories
	if fs.trySkillsOpenDir(op) {
		return nil
	}
	// Handle virtual @providers directories
	if fs.tryProvidersOpenDir(op) {
		return nil
	}
	// Handle virtual @memory directories
	if fs.tryMemoryOpenDir(op) {
		return nil
	}
	// Handle virtual @bridges directories
	if fs.tryBridgesOpenDir(op) {
		return nil
	}
	// Handle virtual @kernel directories
	if fs.tryKernelOpenDir(op) {
		return nil
	}
	// Handle virtual @daemon directories
	if fs.tryDaemonOpenDir(op) {
		return nil
	}
	// Handle virtual @workspaces directories
	if fs.tryWorkspacesOpenDir(op) {
		return nil
	}

	dirPath, err := fs.pathForInode(op.Inode)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return err
	}

	var dirents []fuseutil.Dirent
	for i, e := range entries {
		childPath := filepath.Join(dirPath, e.Name())
		info, err := os.Lstat(childPath)
		if err != nil {
			continue
		}
		rawStat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}
		ino := fuseops.InodeID(rawStat.Ino)
		typ := fuseutil.DT_File
		if info.IsDir() {
			typ = fuseutil.DT_Directory
		} else if info.Mode()&os.ModeSymlink != 0 {
			typ = fuseutil.DT_Link
		}

		fs.mu.Lock()
		fs.inodeMap[ino] = childPath
		fs.mu.Unlock()

		dirents = append(dirents, fuseutil.Dirent{
			Offset: fuseops.DirOffset(i + 1),
			Inode:  ino,
			Name:   e.Name(),
			Type:   typ,
		})
	}

	// Inject virtual files when listing root directory
	if op.Inode == fuseops.RootInodeID {
		dirents = append(dirents, fs.contextDirent(fuseops.DirOffset(len(dirents)+1)))
		dirents = append(dirents, fs.refactorDirent(fuseops.DirOffset(len(dirents)+1))...)
		dirents = append(dirents, fs.searchDirDirent(fuseops.DirOffset(len(dirents)+1)))
		dirents = append(dirents, fs.graphDirDirent(fuseops.DirOffset(len(dirents)+1)))
			dirents = append(dirents, fs.teamDirDirent(fuseops.DirOffset(len(dirents)+1)))
			dirents = append(dirents, fs.tasksDirDirent(fuseops.DirOffset(len(dirents)+1)))
			dirents = append(dirents, fs.skillsDirDirent(fuseops.DirOffset(len(dirents)+1)))
			dirents = append(dirents, fs.providersDirDirent(fuseops.DirOffset(len(dirents)+1)))
			dirents = append(dirents, fs.memoryDirDirent(fuseops.DirOffset(len(dirents)+1)))
			dirents = append(dirents, fs.bridgesDirDirent(fuseops.DirOffset(len(dirents)+1)))
			dirents = append(dirents, fs.kernelDirDirent(fuseops.DirOffset(len(dirents)+1)))
			dirents = append(dirents, fs.daemonDirDirent(fuseops.DirOffset(len(dirents)+1)))
			dirents = append(dirents, fs.workspacesDirDirent(fuseops.DirOffset(len(dirents)+1)))
	}

	handle := fs.allocHandle()
	fs.handleMu.Lock()
	fs.dirs[handle] = &dirHandle{entries: dirents}
	fs.handleMu.Unlock()

	op.Handle = handle
	return nil
}

func (fs *FileSystem) ReadDir(_ context.Context, op *fuseops.ReadDirOp) error {
	fs.handleMu.Lock()
	dh, ok := fs.dirs[op.Handle]
	fs.handleMu.Unlock()
	if !ok {
		return syscall.EBADF
	}

	offset := int(op.Offset)
	if offset > len(dh.entries) {
		return nil
	}

	for i := offset; i < len(dh.entries); i++ {
		n := fuseutil.WriteDirent(op.Dst[op.BytesRead:], dh.entries[i])
		if n == 0 {
			break
		}
		op.BytesRead += n
	}
	return nil
}

func (fs *FileSystem) ReleaseDirHandle(_ context.Context, op *fuseops.ReleaseDirHandleOp) error {
	fs.handleMu.Lock()
	delete(fs.dirs, op.Handle)
	fs.handleMu.Unlock()
	return nil
}

func (fs *FileSystem) MkDir(_ context.Context, op *fuseops.MkDirOp) error {
	if op.Parent == fuseops.RootInodeID && isVirtualFileName(op.Name) {
		return syscall.EPERM
	}
	parentPath, err := fs.pathForInode(op.Parent)
	if err != nil {
		return err
	}
	dirPath := filepath.Join(parentPath, op.Name)

	if err := os.Mkdir(dirPath, op.Mode); err != nil {
		return err
	}
	ino, err := fs.registerPath(dirPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(dirPath)
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

func (fs *FileSystem) Rename(_ context.Context, op *fuseops.RenameOp) error {
	if op.OldParent == fuseops.RootInodeID && isVirtualFileName(op.OldName) {
		return syscall.EPERM
	}
	if op.NewParent == fuseops.RootInodeID && isVirtualFileName(op.NewName) {
		return syscall.EPERM
	}

	oldParent, err := fs.pathForInode(op.OldParent)
	if err != nil {
		return err
	}
	newParent, err := fs.pathForInode(op.NewParent)
	if err != nil {
		return err
	}
	oldPath := filepath.Join(oldParent, op.OldName)
	newPath := filepath.Join(newParent, op.NewName)

	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	if _, err := fs.registerPath(newPath); err != nil {
		return err
	}
	fs.mu.Lock()
	for ino, p := range fs.inodeMap {
		if p == oldPath {
			delete(fs.inodeMap, ino)
		}
	}
	fs.mu.Unlock()
	return nil
}

func (fs *FileSystem) RmDir(_ context.Context, op *fuseops.RmDirOp) error {
	parentPath, err := fs.pathForInode(op.Parent)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(parentPath, op.Name))
}

func (fs *FileSystem) Unlink(_ context.Context, op *fuseops.UnlinkOp) error {
	if op.Parent == fuseops.RootInodeID && isVirtualFileName(op.Name) {
		return syscall.EPERM
	}
	parentPath, err := fs.pathForInode(op.Parent)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(parentPath, op.Name))
}
