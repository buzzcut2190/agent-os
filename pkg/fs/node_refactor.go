package fs

import (
	"fmt"
	"os"
	"time"

	"github.com/agent-os/agent-os/pkg/refactor"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

// Virtual inode IDs for the @refactor/* synthetic files.
// Chosen outside the range returned by real filesystem lookups.
const (
	refactorDeadCodeInode   fuseops.InodeID = 0xFFFFF200
	refactorComplexityInode fuseops.InodeID = 0xFFFFF201
	refactorLintInode       fuseops.InodeID = 0xFFFFF202
)

// Virtual file names visible under the root directory.
const (
	refactorDeadCodeName   = "@dead-code"
	refactorComplexityName = "@complexity"
	refactorLintName       = "@lint"
)

// refactorEngine is lazily initialised and groups the refactoring tools
// that produce the content for the three virtual files.
type refactorEngine struct {
	deadCode   *refactor.GoDeadCodeDetector
	complexity *refactor.GoComplexityCalculator
}

// refactorEngineLazy initialises the refactor engine exactly once.
func (fs *FileSystem) refactorEngineLazy() *refactorEngine {
	fs.refactorOnce.Do(func() {
		fs.refactorEng = &refactorEngine{
			deadCode:   refactor.NewDeadCodeDetector(),
			complexity: refactor.NewComplexityCalculator(),
		}
	})
	return fs.refactorEng
}

// refactorDirent returns the dirent for each of the three virtual files.
func (fs *FileSystem) refactorDirent(offset fuseops.DirOffset) []fuseutil.Dirent {
	return []fuseutil.Dirent{
		{Offset: offset, Inode: refactorDeadCodeInode, Name: refactorDeadCodeName, Type: fuseutil.DT_File},
		{Offset: offset + 1, Inode: refactorComplexityInode, Name: refactorComplexityName, Type: fuseutil.DT_File},
		{Offset: offset + 2, Inode: refactorLintInode, Name: refactorLintName, Type: fuseutil.DT_File},
	}
}

// isRefactorInode reports whether the given inode belongs to a refactor
// virtual file.
func (fs *FileSystem) isRefactorInode(inode fuseops.InodeID) bool {
	switch inode {
	case refactorDeadCodeInode, refactorComplexityInode, refactorLintInode:
		return true
	}
	return false
}

// isVirtualFileName returns true for any name that maps to a synthetic virtual
// file under the root directory (both @context and the @refactor/* files).
func isVirtualFileName(name string) bool {
	switch name {
	case contextName, refactorDeadCodeName, refactorComplexityName, refactorLintName,
		searchDirName, graphDirName, teamDirName, tasksDirName, skillsDirName, providersDirName, memoryDirName, bridgesDirName, kernelDirName, daemonDirName, workspacesDirName:
		return true
	}
	return false
}

// tryRefactorLookup handles name lookup for the three refactor virtual files.
func (fs *FileSystem) tryRefactorLookup(name string) (*fuseops.ChildInodeEntry, bool) {
	var ino fuseops.InodeID
	switch name {
	case refactorDeadCodeName:
		ino = refactorDeadCodeInode
	case refactorComplexityName:
		ino = refactorComplexityInode
	case refactorLintName:
		ino = refactorLintInode
	default:
		return nil, false
	}

	size := fs.refactorFileSize(ino)
	return &fuseops.ChildInodeEntry{
		Child:      ino,
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

// tryRefactorGetAttr returns attributes for a refactor virtual inode.
func (fs *FileSystem) tryRefactorGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isRefactorInode(op.Inode) {
		return false
	}
	size := fs.refactorFileSize(op.Inode)
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

// tryRefactorRead serves the content of a refactor virtual file.
func (fs *FileSystem) tryRefactorRead(op *fuseops.ReadFileOp) bool {
	if !fs.isRefactorInode(op.Inode) {
		return false
	}

	var data []byte
	var err error

	switch op.Inode {
	case refactorDeadCodeInode:
		data, err = fs.deadCodeReport()
	case refactorComplexityInode:
		data, err = fs.complexityReport()
	case refactorLintInode:
		data, err = fs.lintReport()
	default:
		return false
	}

	if err != nil {
		data = []byte(fmt.Sprintf("error: %v\n", err))
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

// refactorFileSize returns the size in bytes for a refactor virtual file.
func (fs *FileSystem) refactorFileSize(inode fuseops.InodeID) uint64 {
	var data []byte
	var err error

	switch inode {
	case refactorDeadCodeInode:
		data, err = fs.deadCodeReport()
	case refactorComplexityInode:
		data, err = fs.complexityReport()
	case refactorLintInode:
		data, err = fs.lintReport()
	default:
		return 0
	}
	if err != nil {
		data = []byte(fmt.Sprintf("error: %v\n", err))
	}
	return uint64(len(data))
}

