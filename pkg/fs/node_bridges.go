package fs

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agent-os/agent-os/pkg/bridge"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	bridgesDirName = "@bridges"

	bridgesInodeBase   fuseops.InodeID = 0xFFFFF800
	bridgesDirInode                    = bridgesInodeBase + 0x000
	bridgesStatusInode                 = bridgesInodeBase + 0x001
	bridgesRegisterInode               = bridgesInodeBase + 0x002
)

var bridgesStaticChildren = map[string]struct {
	inode fuseops.InodeID
	isDir bool
}{
	"status":   {bridgesStatusInode, false},
	"register": {bridgesRegisterInode, false},
}

var globalBridgeRegistry *bridge.Registry

// SetBridgeRegistry injects the bridge registry for FUSE access.
func SetBridgeRegistry(reg *bridge.Registry) {
	globalBridgeRegistry = reg
}

func (fs *FileSystem) isBridgesInode(inode fuseops.InodeID) bool {
	return inode >= bridgesInodeBase && inode <= bridgesRegisterInode
}

func (fs *FileSystem) bridgesDirDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{
		Offset: offset, Inode: bridgesDirInode,
		Name: bridgesDirName, Type: fuseutil.DT_Directory,
	}
}

func (fs *FileSystem) tryBridgesLookup(name string, parent fuseops.InodeID) (*fuseops.ChildInodeEntry, bool) {
	switch {
	case parent == fuseops.RootInodeID && name == bridgesDirName:
		return fs.bridgesChildEntry(bridgesDirInode, true), true
	case parent == bridgesDirInode:
		if c, ok := bridgesStaticChildren[name]; ok {
			return fs.bridgesChildEntry(c.inode, c.isDir), true
		}
		// Bridge subdirectories (feishu, webhook, etc.)
		if globalBridgeRegistry != nil {
			for _, info := range globalBridgeRegistry.All() {
				if name == info.Name {
					return fs.bridgesChildEntry(bridgesDirInode|0x100|hashInode(name)&0xFF, true), true
				}
			}
		}
	}
	return nil, false
}

func (fs *FileSystem) bridgesChildEntry(ino fuseops.InodeID, isDir bool) *fuseops.ChildInodeEntry {
	return &fuseops.ChildInodeEntry{
		Child: ino, Generation: 1,
		Attributes:           fs.bridgesAttr(ino, isDir),
		AttributesExpiration: time.Now().Add(5 * time.Second),
		EntryExpiration:      time.Now().Add(5 * time.Second),
	}
}

func (fs *FileSystem) tryBridgesGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isBridgesInode(op.Inode) {
		return false
	}
	isDir := fs.bridgesIsDir(op.Inode)
	op.Attributes = fs.bridgesAttr(op.Inode, isDir)
	op.AttributesExpiration = time.Now().Add(5 * time.Second)
	return true
}

func (fs *FileSystem) tryBridgesRead(op *fuseops.ReadFileOp) bool {
	if !fs.isBridgesInode(op.Inode) {
		return false
	}
	var data []byte
	switch op.Inode {
	case bridgesStatusInode:
		if globalBridgeRegistry != nil {
			var b strings.Builder
			b.WriteString("Bridge Status\n")
			b.WriteString(strings.Repeat("-", 40) + "\n")
			for _, info := range globalBridgeRegistry.All() {
				fmt.Fprintf(&b, "%-15s %-12s %s\n", info.Name, info.Type, info.Status)
			}
			data = []byte(b.String())
		} else {
			data = []byte("No bridges configured.\n")
		}
	case bridgesRegisterInode:
		data = []byte("# Register a new bridge\n\necho '{\"type\":\"webhook\",\"name\":\"my-webhook\",\"webhook_url\":\"...\"}' > @bridges/register\n")
	default:
		return false
	}
	if op.Offset >= int64(len(data)) {
		op.BytesRead = 0
		return true
	}
	end := int(op.Offset) + len(op.Dst)
	if end > len(data) {
		end = len(data)
	}
	op.BytesRead = copy(op.Dst, data[op.Offset:end])
	return true
}

func (fs *FileSystem) tryBridgesWrite(op *fuseops.WriteFileOp) bool {
	if !fs.isBridgesInode(op.Inode) {
		return false
	}
	if op.Inode == bridgesRegisterInode && op.Offset == 0 {
		// Registration acknowledged.
		return true
	}
	return false
}

func (fs *FileSystem) tryBridgesOpenDir(op *fuseops.OpenDirOp) bool {
	if !fs.isBridgesInode(op.Inode) {
		return false
	}
	if !fs.bridgesIsDir(op.Inode) {
		return false
	}
	entries := make([]fuseutil.Dirent, 0)
	o := 0
	add := func(ino fuseops.InodeID, name string, isDir bool) {
		o++
		entries = append(entries, fuseutil.Dirent{
			Offset: fuseops.DirOffset(o), Inode: ino, Name: name,
			Type: bridgesDirentType(isDir),
		})
	}
	switch op.Inode {
	case bridgesDirInode:
		add(bridgesStatusInode, "status", false)
		add(bridgesRegisterInode, "register", false)
		if globalBridgeRegistry != nil {
			for _, info := range globalBridgeRegistry.All() {
				add(bridgesDirInode|0x100|hashInode(info.Name)&0xFF, info.Name, true)
			}
		}
	default:
		return false
	}
	handle := fs.allocHandle()
	fs.handleMu.Lock()
	fs.dirs[handle] = &dirHandle{entries: entries}
	fs.handleMu.Unlock()
	op.Handle = handle
	return true
}

func (fs *FileSystem) bridgesAttr(inode fuseops.InodeID, isDir bool) fuseops.InodeAttributes {
	mode := os.FileMode(0444)
	var size uint64
	if isDir {
		mode, size = 0555|os.ModeDir, 4096
	} else {
		if inode == bridgesRegisterInode {
			mode = 0644
		}
	}
	return fuseops.InodeAttributes{
		Size: size, Nlink: 1, Mode: mode,
		Atime: time.Now(), Mtime: time.Now(), Ctime: time.Now(),
		Uid: uint32(os.Getuid()), Gid: uint32(os.Getgid()),
	}
}

func (fs *FileSystem) bridgesIsDir(inode fuseops.InodeID) bool {
	return inode == bridgesDirInode || (inode > bridgesDirInode && inode < bridgesDirInode+0x1000 && inode != bridgesStatusInode && inode != bridgesRegisterInode)
}

func bridgesDirentType(isDir bool) fuseutil.DirentType {
	if isDir {
		return fuseutil.DT_Directory
	}
	return fuseutil.DT_File
}

func (fs *FileSystem) isBridgesStateWritable(inode fuseops.InodeID) bool {
	return inode == bridgesRegisterInode
}
