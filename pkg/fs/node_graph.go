package fs

import (
	"fmt"
	"os"
	"time"

	"github.com/agent-os/agent-os/pkg/index"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	graphDirName = "@graph"

	graphDirInode          fuseops.InodeID = 0xFFFFF100
	graphDependenciesInode fuseops.InodeID = 0xFFFFF101
	graphCallGraphInode    fuseops.InodeID = 0xFFFFF102
	graphImportMapInode    fuseops.InodeID = 0xFFFFF103
)

// Static name-to-inode routing for @graph/ contents.
var graphFileNames = map[string]fuseops.InodeID{
	"deps.dot":      graphDependenciesInode,
	"callgraph.dot": graphCallGraphInode,
	"importmap.dot": graphImportMapInode,
}

func (fs *FileSystem) graphDirDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{
		Offset: offset,
		Inode:  graphDirInode,
		Name:   graphDirName,
		Type:   fuseutil.DT_Directory,
	}
}

func (fs *FileSystem) isGraphInode(inode fuseops.InodeID) bool {
	return inode >= graphDirInode && inode <= graphImportMapInode
}

func (fs *FileSystem) tryGraphLookup(name string, parent fuseops.InodeID) (*fuseops.ChildInodeEntry, bool) {
	var ino fuseops.InodeID
	var isDir bool

	switch {
	case parent == fuseops.RootInodeID && name == graphDirName:
		ino = graphDirInode
		isDir = true
	case parent == graphDirInode:
		if id, ok := graphFileNames[name]; ok {
			ino = id
		} else {
			return nil, false
		}
	default:
		return nil, false
	}

	return &fuseops.ChildInodeEntry{
		Child:      ino,
		Generation: 1,
		Attributes: fs.graphAttr(ino, isDir),
		AttributesExpiration: time.Now().Add(time.Second),
		EntryExpiration:      time.Now().Add(time.Second),
	}, true
}

func (fs *FileSystem) tryGraphGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isGraphInode(op.Inode) {
		return false
	}
	isDir := op.Inode == graphDirInode
	op.Attributes = fs.graphAttr(op.Inode, isDir)
	op.AttributesExpiration = time.Now().Add(time.Second)
	return true
}

func (fs *FileSystem) tryGraphRead(op *fuseops.ReadFileOp) bool {
	if !fs.isGraphInode(op.Inode) || op.Inode == graphDirInode {
		return false
	}
	data, err := fs.graphContent(op.Inode)
	if err != nil {
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

func (fs *FileSystem) tryGraphOpenDir(op *fuseops.OpenDirOp) bool {
	if op.Inode != graphDirInode {
		return false
	}
	entries := []fuseutil.Dirent{
		{Offset: 1, Inode: graphDependenciesInode, Name: "deps.dot", Type: fuseutil.DT_File},
		{Offset: 2, Inode: graphCallGraphInode, Name: "callgraph.dot", Type: fuseutil.DT_File},
		{Offset: 3, Inode: graphImportMapInode, Name: "importmap.dot", Type: fuseutil.DT_File},
	}
	handle := fs.allocHandle()
	fs.handleMu.Lock()
	fs.dirs[handle] = &dirHandle{entries: entries}
	fs.handleMu.Unlock()
	op.Handle = handle
	return true
}

func (fs *FileSystem) graphAttr(inode fuseops.InodeID, isDir bool) fuseops.InodeAttributes {
	mode := os.FileMode(0444)
	if isDir {
		mode = 0555 | os.ModeDir
	}
	return fuseops.InodeAttributes{
		Size:  fs.graphSize(inode, isDir),
		Nlink: 1,
		Mode:  mode,
		Atime: time.Now(),
		Mtime: time.Now(),
		Ctime: time.Now(),
		Uid:   uint32(os.Getuid()),
		Gid:   uint32(os.Getgid()),
	}
}

func (fs *FileSystem) graphSize(inode fuseops.InodeID, isDir bool) uint64 {
	if isDir {
		return 4096
	}
	data, err := fs.graphContent(inode)
	if err != nil {
		return 0
	}
	return uint64(len(data))
}

func (fs *FileSystem) graphContent(inode fuseops.InodeID) ([]byte, error) {
	result, err := fs.indexEngine().GetAnalysis()
	if err != nil {
		return nil, err
	}
	switch inode {
	case graphDependenciesInode:
		return index.DepsDot(result), nil
	case graphCallGraphInode:
		return index.CallGraphDot(result), nil
	case graphImportMapInode:
		return index.ImportMapDot(result), nil
	default:
		return nil, fmt.Errorf("unknown graph inode %d", inode)
	}
}
