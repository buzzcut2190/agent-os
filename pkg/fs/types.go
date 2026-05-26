package fs

import (
	"os"
	"sync"

	"github.com/agent-os/agent-os/pkg/context"
	"github.com/agent-os/agent-os/pkg/index"
	"github.com/agent-os/agent-os/pkg/skill"
	"github.com/agent-os/agent-os/pkg/team"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

// FileSystem implements a FUSE passthrough file system.
// It proxies all operations to an underlying source directory.
type FileSystem struct {
	fuseutil.NotImplementedFileSystem

	sourceDir string
	mu        sync.RWMutex
	inodeMap  map[fuseops.InodeID]string // FUSE inode -> absolute path
	rootInode fuseops.InodeID            // real inode of source dir

	handleMu   sync.Mutex
	nextHandle fuseops.HandleID
	files      map[fuseops.HandleID]*os.File
	dirs       map[fuseops.HandleID]*dirHandle

	// @context virtual file
	ctxOnce   sync.Once
	ctxEngine *context.Engine

	// @refactor virtual files
	refactorOnce sync.Once
	refactorEng  *refactorEngine

	// @search and @graph virtual files
	indexOnce sync.Once
	indexEng  *index.Engine

	// @team and @tasks virtual files
	teamOnce sync.Once
	teamEng  *team.TeamStore

	// @skills virtual files
	skillOnce sync.Once
	skillEng  *skill.Engine
	skillMu   sync.RWMutex
}

// dirHandle tracks an open directory's entries for ReadDir.
type dirHandle struct {
	entries []fuseutil.Dirent
}

