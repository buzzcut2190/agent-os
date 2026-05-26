package fs

import (
	"os"
	"time"

	"github.com/agent-os/agent-os/pkg/context"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	contextInode fuseops.InodeID = 0xFFFFFFFF
	contextName                  = "@context"
)

func (fs *FileSystem) contextEngine() *context.Engine {
	fs.ctxOnce.Do(func() {
		fs.ctxEngine = context.NewEngine(fs.sourceDir)
	})
	return fs.ctxEngine
}

func (fs *FileSystem) contextContent() ([]byte, error) {
	summary, err := fs.contextEngine().GetSummary()
	if err != nil {
		return nil, err
	}
	return []byte(context.FormatMarkdown(summary)), nil
}

func (fs *FileSystem) contextSize() uint64 {
	data, err := fs.contextContent()
	if err != nil {
		return 0
	}
	return uint64(len(data))
}

func (fs *FileSystem) contextDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{
		Offset: offset,
		Inode:  contextInode,
		Name:   contextName,
		Type:   fuseutil.DT_File,
	}
}

func (fs *FileSystem) tryContextLookup(name string) (*fuseops.ChildInodeEntry, bool) {
	if name != contextName {
		return nil, false
	}
	size := fs.contextSize()
	return &fuseops.ChildInodeEntry{
		Child:      contextInode,
		Generation: 1,
		Attributes: fuseops.InodeAttributes{
			Size:   size,
			Nlink:  1,
			Mode:   0444,
			Atime:  time.Now(),
			Mtime:  time.Now(),
			Ctime:  time.Now(),
			Uid:    uint32(os.Getuid()),
			Gid:    uint32(os.Getgid()),
		},
		AttributesExpiration: time.Now().Add(time.Second),
		EntryExpiration:      time.Now().Add(time.Second),
	}, true
}

func (fs *FileSystem) tryContextGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if op.Inode != contextInode {
		return false
	}
	size := fs.contextSize()
	op.Attributes = fuseops.InodeAttributes{
		Size:   size,
		Nlink:  1,
		Mode:   0444,
		Atime:  time.Now(),
		Mtime:  time.Now(),
		Ctime:  time.Now(),
		Uid:    uint32(os.Getuid()),
		Gid:    uint32(os.Getgid()),
	}
	op.AttributesExpiration = time.Now().Add(time.Second)
	return true
}

func (fs *FileSystem) tryContextRead(op *fuseops.ReadFileOp) bool {
	if op.Inode != contextInode {
		return false
	}
	data, err := fs.contextContent()
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

func (fs *FileSystem) isContextInode(inode fuseops.InodeID) bool {
	return inode == contextInode
}
