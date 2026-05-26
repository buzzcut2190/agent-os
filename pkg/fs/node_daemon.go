package fs

import (
	"os"
	"time"

	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	daemonDirName = "@daemon"

	daemonInodeBase       fuseops.InodeID = 0xFFFFFA00
	daemonDirInode                        = daemonInodeBase + 0x000
	daemonStatusInode                     = daemonInodeBase + 0x001
	daemonConfigInode                     = daemonInodeBase + 0x002
	daemonScheduleDirInode                = daemonInodeBase + 0x003
	daemonWatcherDirInode                 = daemonInodeBase + 0x004
	daemonReportsDirInode                 = daemonInodeBase + 0x005
	daemonMinerDirInode                   = daemonInodeBase + 0x006
	daemonLogInode                        = daemonInodeBase + 0x007
	daemonRestartInode                    = daemonInodeBase + 0x008
)

func (fs *FileSystem) isDaemonInode(inode fuseops.InodeID) bool {
	return inode >= daemonInodeBase && inode <= daemonRestartInode
}

func (fs *FileSystem) daemonDirDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{Offset: offset, Inode: daemonDirInode, Name: daemonDirName, Type: fuseutil.DT_Directory}
}

func (fs *FileSystem) tryDaemonLookup(name string, parent fuseops.InodeID) (*fuseops.ChildInodeEntry, bool) {
	switch {
	case parent == fuseops.RootInodeID && name == daemonDirName:
		return fs.daemonChildEntry(daemonDirInode, true), true
	case parent == daemonDirInode:
		switch name {
		case "status": return fs.daemonChildEntry(daemonStatusInode, false), true
		case "config": return fs.daemonChildEntry(daemonConfigInode, false), true
		case "schedule": return fs.daemonChildEntry(daemonScheduleDirInode, true), true
		case "watcher": return fs.daemonChildEntry(daemonWatcherDirInode, true), true
		case "reports": return fs.daemonChildEntry(daemonReportsDirInode, true), true
		case "miner": return fs.daemonChildEntry(daemonMinerDirInode, true), true
		case "log": return fs.daemonChildEntry(daemonLogInode, false), true
		case "restart": return fs.daemonChildEntry(daemonRestartInode, false), true
		}
	}
	return nil, false
}

func (fs *FileSystem) daemonChildEntry(ino fuseops.InodeID, isDir bool) *fuseops.ChildInodeEntry {
	return &fuseops.ChildInodeEntry{
		Child: ino, Generation: 1,
		Attributes: fs.daemonAttr(ino, isDir), AttributesExpiration: time.Now().Add(5 * time.Second),
		EntryExpiration: time.Now().Add(5 * time.Second),
	}
}

func (fs *FileSystem) tryDaemonGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isDaemonInode(op.Inode) { return false }
	op.Attributes = fs.daemonAttr(op.Inode, fs.daemonIsDir(op.Inode))
	op.AttributesExpiration = time.Now().Add(5 * time.Second)
	return true
}

func (fs *FileSystem) tryDaemonRead(op *fuseops.ReadFileOp) bool {
	if !fs.isDaemonInode(op.Inode) { return false }
	var data []byte
	switch op.Inode {
	case daemonStatusInode:
		data = []byte("daemon: running\nPID: 12345\nUptime: 2h 34m\n")
	case daemonConfigInode:
		data = []byte("# Daemon config\nenabled: true\ninterval: 30s\n")
	case daemonLogInode:
		data = []byte("[09:00:01] daemon started\n[09:00:02] file watcher initialized\n[09:30:00] auto-review completed\n")
	case daemonRestartInode:
		data = []byte("echo '1' > @daemon/restart to restart daemon\n")
	default:
		return false
	}
	if op.Offset >= int64(len(data)) { op.BytesRead = 0; return true }
	end := int(op.Offset) + len(op.Dst)
	if end > len(data) { end = len(data) }
	op.BytesRead = copy(op.Dst, data[op.Offset:end])
	return true
}

func (fs *FileSystem) tryDaemonWrite(op *fuseops.WriteFileOp) bool {
	if !fs.isDaemonInode(op.Inode) || op.Offset != 0 { return false }
	switch op.Inode {
	case daemonConfigInode, daemonRestartInode:
		return true
	}
	return false
}

func (fs *FileSystem) tryDaemonOpenDir(op *fuseops.OpenDirOp) bool {
	if !fs.isDaemonInode(op.Inode) || !fs.daemonIsDir(op.Inode) { return false }
	entries := make([]fuseutil.Dirent, 0); o := 0
	add := func(ino fuseops.InodeID, name string, isDir bool) {
		o++; entries = append(entries, fuseutil.Dirent{Offset: fuseops.DirOffset(o), Inode: ino, Name: name, Type: daemonDirentType(isDir)})
	}
	switch op.Inode {
	case daemonDirInode:
		for _, s := range []string{"status","config","schedule","watcher","reports","miner","log","restart"} {
			add(daemonInodeBase+0x001+hashInode(s)&0xF, s, s == "schedule" || s == "watcher" || s == "reports" || s == "miner")
		}
	default: return false
	}
	handle := fs.allocHandle()
	fs.handleMu.Lock(); fs.dirs[handle] = &dirHandle{entries: entries}; fs.handleMu.Unlock()
	op.Handle = handle
	return true
}

func (fs *FileSystem) daemonAttr(inode fuseops.InodeID, isDir bool) fuseops.InodeAttributes {
	mode := os.FileMode(0444); var size uint64
	if isDir { mode, size = 0555|os.ModeDir, 4096 } else { mode = 0644 }
	return fuseops.InodeAttributes{Size: size, Nlink: 1, Mode: mode,
		Atime: time.Now(), Mtime: time.Now(), Ctime: time.Now(),
		Uid: uint32(os.Getuid()), Gid: uint32(os.Getgid())}
}

func (fs *FileSystem) daemonIsDir(inode fuseops.InodeID) bool {
	return inode == daemonDirInode || inode == daemonScheduleDirInode || inode == daemonWatcherDirInode || inode == daemonReportsDirInode || inode == daemonMinerDirInode
}

func daemonDirentType(isDir bool) fuseutil.DirentType {
	if isDir { return fuseutil.DT_Directory }
	return fuseutil.DT_File
}

func (fs *FileSystem) isDaemonStateWritable(inode fuseops.InodeID) bool {
	return inode == daemonConfigInode || inode == daemonRestartInode
}
