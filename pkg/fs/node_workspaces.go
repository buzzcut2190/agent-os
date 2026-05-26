package fs

import (
	"fmt"
	"os"
	"time"

	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	workspacesDirName = "@workspaces"

	workspacesInodeBase      fuseops.InodeID = 0xFFFFFB00
	workspacesDirInode                        = workspacesInodeBase + 0x000
	workspacesCreateInode                     = workspacesInodeBase + 0x001
	workspacesSearchInode                     = workspacesInodeBase + 0x002
	workspacesStatsInode                      = workspacesInodeBase + 0x003
	workspacesMeInode                         = workspacesInodeBase + 0x004

	dynWorkspaceBase fuseops.InodeID = 0xFF700000
)

func (fs *FileSystem) isWorkspacesInode(inode fuseops.InodeID) bool {
	return (inode >= workspacesInodeBase && inode <= workspacesMeInode) ||
		(inode >= dynWorkspaceBase && inode < dynWorkspaceBase+0x100000)
}

func (fs *FileSystem) workspacesDirDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{Offset: offset, Inode: workspacesDirInode, Name: workspacesDirName, Type: fuseutil.DT_Directory}
}

func (fs *FileSystem) tryWorkspacesLookup(name string, parent fuseops.InodeID) (*fuseops.ChildInodeEntry, bool) {
	switch {
	case parent == fuseops.RootInodeID && name == workspacesDirName:
		return fs.workspacesChildEntry(workspacesDirInode, true), true
	case parent == workspacesDirInode:
		switch name {
		case "create": return fs.workspacesChildEntry(workspacesCreateInode, false), true
		case "search": return fs.workspacesChildEntry(workspacesSearchInode, false), true
		case "stats": return fs.workspacesChildEntry(workspacesStatsInode, false), true
		case "me": return fs.workspacesChildEntry(workspacesMeInode, true), true
		}
	}
	return nil, false
}

func (fs *FileSystem) workspacesChildEntry(ino fuseops.InodeID, isDir bool) *fuseops.ChildInodeEntry {
	return &fuseops.ChildInodeEntry{
		Child: ino, Generation: 1,
		Attributes: fs.workspacesAttr(ino, isDir), AttributesExpiration: time.Now().Add(5 * time.Second),
		EntryExpiration: time.Now().Add(5 * time.Second),
	}
}

func (fs *FileSystem) tryWorkspacesGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isWorkspacesInode(op.Inode) { return false }
	op.Attributes = fs.workspacesAttr(op.Inode, fs.workspacesIsDir(op.Inode))
	op.AttributesExpiration = time.Now().Add(5 * time.Second)
	return true
}

func (fs *FileSystem) tryWorkspacesRead(op *fuseops.ReadFileOp) bool {
	if !fs.isWorkspacesInode(op.Inode) { return false }
	var data []byte
	switch op.Inode {
	case workspacesCreateInode:
		data = []byte("# Create Workspace\ncat > @workspaces/create <<EOF\n{\"type\":\"agent\",\"owner\":\"me\"}\nEOF\n")
	case workspacesSearchInode:
		data = []byte("# Search Workspaces\necho \"query\" > @workspaces/search to search\n")
	case workspacesStatsInode:
		data = []byte(fmt.Sprintf("Total workspaces: 0\nActive: 0\nArchived: 0\n"))
	default:
		return false
	}
	if op.Offset >= int64(len(data)) { op.BytesRead = 0; return true }
	end := int(op.Offset) + len(op.Dst)
	if end > len(data) { end = len(data) }
	op.BytesRead = copy(op.Dst, data[op.Offset:end])
	return true
}

func (fs *FileSystem) tryWorkspacesWrite(op *fuseops.WriteFileOp) bool {
	if !fs.isWorkspacesInode(op.Inode) || op.Offset != 0 { return false }
	switch op.Inode {
	case workspacesCreateInode, workspacesSearchInode:
		return true
	}
	return false
}

func (fs *FileSystem) tryWorkspacesOpenDir(op *fuseops.OpenDirOp) bool {
	if !fs.isWorkspacesInode(op.Inode) || !fs.workspacesIsDir(op.Inode) { return false }
	entries := make([]fuseutil.Dirent, 0); o := 0
	add := func(ino fuseops.InodeID, name string, isDir bool) {
		o++; entries = append(entries, fuseutil.Dirent{Offset: fuseops.DirOffset(o), Inode: ino, Name: name, Type: workspacesDirentType(isDir)})
	}
	switch op.Inode {
	case workspacesDirInode:
		add(workspacesCreateInode, "create", false)
		add(workspacesSearchInode, "search", false)
		add(workspacesStatsInode, "stats", false)
		add(workspacesMeInode, "me", true)
	default: return false
	}
	handle := fs.allocHandle()
	fs.handleMu.Lock(); fs.dirs[handle] = &dirHandle{entries: entries}; fs.handleMu.Unlock()
	op.Handle = handle
	return true
}

func (fs *FileSystem) workspacesAttr(inode fuseops.InodeID, isDir bool) fuseops.InodeAttributes {
	mode := os.FileMode(0444); var size uint64
	if isDir { mode, size = 0555|os.ModeDir, 4096 } else { mode = 0644 }
	return fuseops.InodeAttributes{Size: size, Nlink: 1, Mode: mode,
		Atime: time.Now(), Mtime: time.Now(), Ctime: time.Now(),
		Uid: uint32(os.Getuid()), Gid: uint32(os.Getgid())}
}

func (fs *FileSystem) workspacesIsDir(inode fuseops.InodeID) bool {
	return inode == workspacesDirInode || inode == workspacesMeInode
}

func workspacesDirentType(isDir bool) fuseutil.DirentType {
	if isDir { return fuseutil.DT_Directory }
	return fuseutil.DT_File
}

func (fs *FileSystem) isWorkspacesStateWritable(inode fuseops.InodeID) bool {
	return inode == workspacesCreateInode || inode == workspacesSearchInode
}
