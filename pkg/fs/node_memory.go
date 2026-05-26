package fs

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agent-os/agent-os/pkg/memory"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	memoryDirName = "@memory"

	memoryInodeBase         fuseops.InodeID = 0xFFFFF700
	memoryDirInode                          = memoryInodeBase + 0x000
	memorySessionsInode                     = memoryInodeBase + 0x001
	memoryDecisionsInode                    = memoryInodeBase + 0x002
	memoryPreferencesInode                  = memoryInodeBase + 0x003
	memoryKnowledgeInode                    = memoryInodeBase + 0x004
	memorySearchInode                       = memoryInodeBase + 0x005
	memoryNoteInode                         = memoryInodeBase + 0x006
	memoryRecentInode                       = memoryInodeBase + 0x007
	memoryStatsInode                        = memoryInodeBase + 0x008
	memoryForgetInode                       = memoryInodeBase + 0x009
	memorySearchResultInode                 = memoryInodeBase + 0x00A
)

var memoryStaticChildren = map[string]struct {
	inode fuseops.InodeID
	isDir bool
}{
	"sessions":    {memorySessionsInode, true},
	"decisions":   {memoryDecisionsInode, true},
	"preferences": {memoryPreferencesInode, true},
	"knowledge":   {memoryKnowledgeInode, true},
	"search":      {memorySearchInode, false},
	"note":        {memoryNoteInode, false},
	"recent":      {memoryRecentInode, false},
	"stats":       {memoryStatsInode, false},
	"forget":      {memoryForgetInode, false},
}

var memoryStaticDirentOrder = []string{"sessions", "decisions", "preferences", "knowledge", "search", "note", "recent", "stats", "forget"}

var globalMemoryStore *memory.Store

// SetMemoryStore injects the memory store for FUSE access.
func SetMemoryStore(store *memory.Store) {
	globalMemoryStore = store
}

func (fs *FileSystem) isMemoryInode(inode fuseops.InodeID) bool {
	return inode >= memoryInodeBase && inode <= memorySearchResultInode
}

func (fs *FileSystem) memoryDirDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{
		Offset: offset, Inode: memoryDirInode,
		Name: memoryDirName, Type: fuseutil.DT_Directory,
	}
}

func (fs *FileSystem) tryMemoryLookup(name string, parent fuseops.InodeID) (*fuseops.ChildInodeEntry, bool) {
	if globalMemoryStore == nil {
		return nil, false
	}
	switch {
	case parent == fuseops.RootInodeID && name == memoryDirName:
		return fs.memoryChildEntry(memoryDirInode, true), true
	case parent == memoryDirInode:
		if c, ok := memoryStaticChildren[name]; ok {
			return fs.memoryChildEntry(c.inode, c.isDir), true
		}
	case parent == memorySearchInode:
		// name becomes the query.
		return nil, false
	}
	return nil, false
}

func (fs *FileSystem) memoryChildEntry(ino fuseops.InodeID, isDir bool) *fuseops.ChildInodeEntry {
	return &fuseops.ChildInodeEntry{
		Child: ino, Generation: 1,
		Attributes:           fs.memoryAttr(ino, isDir),
		AttributesExpiration: time.Now().Add(5 * time.Second),
		EntryExpiration:      time.Now().Add(5 * time.Second),
	}
}

func (fs *FileSystem) tryMemoryGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isMemoryInode(op.Inode) {
		return false
	}
	isDir := fs.memoryIsDir(op.Inode)
	op.Attributes = fs.memoryAttr(op.Inode, isDir)
	op.AttributesExpiration = time.Now().Add(5 * time.Second)
	return true
}

func (fs *FileSystem) tryMemoryRead(op *fuseops.ReadFileOp) bool {
	if !fs.isMemoryInode(op.Inode) {
		return false
	}
	s := globalMemoryStore
	var data []byte

	switch {
	case op.Inode == memoryRecentInode:
		mems, _ := s.Recent(20)
		var b strings.Builder
		for _, m := range mems {
			fmt.Fprintf(&b, "[%s] [%s] %s\n", m.Timestamp.Format("2006-01-02 15:04"), m.Type, truncateContent(m.Content, 200))
		}
		data = []byte(b.String())
	case op.Inode == memoryStatsInode:
		stats, _ := s.Stats()
		data = []byte(fmt.Sprintf("Total: %d\nSessions: %d\nDecisions: %d\nPreferences: %d\nKnowledge: %d\nEarliest: %s\nLatest: %s\n",
			stats.Total, stats.ByType[memory.MemSession], stats.ByType[memory.MemDecision],
			stats.ByType[memory.MemPreference], stats.ByType[memory.MemKnowledge],
			stats.Earliest.Format("2006-01-02"), stats.Latest.Format("2006-01-02")))
	case op.Inode == memoryNoteInode:
		data = []byte("# Quick Note\n\nWrite here to save a note.\n")
	case op.Inode == memorySearchInode:
		data = []byte("# Memory Search\n\necho \"query\" > @memory/search to search.\nThen cat @memory/search/result\n")
	case op.Inode == memorySearchResultInode:
		data = []byte("(no search yet)\n")
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

func (fs *FileSystem) tryMemoryWrite(op *fuseops.WriteFileOp) bool {
	if !fs.isMemoryInode(op.Inode) {
		return false
	}
	s := globalMemoryStore
	if s == nil {
		return false
	}
	if op.Offset != 0 {
		return true
	}
	content := strings.TrimSpace(string(op.Data))

	switch {
	case op.Inode == memoryNoteInode:
		s.Save(memory.Memory{
			Type:    memory.MemKnowledge,
			Source:  "user",
			Content: content,
			Tags:    []string{"note"},
		})
		return true
	case op.Inode == memorySearchInode:
		results, _ := s.Search(memory.SearchOpts{Query: content, Limit: 20})
		var b strings.Builder
		for i, m := range results {
			fmt.Fprintf(&b, "%d. [%s] [%s] %s\n   %s\n\n",
				i+1, m.Timestamp.Format("2006-01-02"), m.Type, strings.Join(m.Tags, ","), m.Content)
		}
		// Store result for reading.
		globalMemorySearchResult = b.String()
		return true
	case op.Inode == memoryForgetInode:
		if content == "all" {
			s.Forget(time.Now().Add(24*time.Hour), memory.MemSession, memory.MemDecision, memory.MemPreference, memory.MemKnowledge)
		}
		return true
	default:
		return false
	}
}

var globalMemorySearchResult string

func (fs *FileSystem) tryMemoryOpenDir(op *fuseops.OpenDirOp) bool {
	if !fs.isMemoryInode(op.Inode) {
		return false
	}
	if !fs.memoryIsDir(op.Inode) {
		return false
	}
	entries := make([]fuseutil.Dirent, 0)
	o := 0
	add := func(ino fuseops.InodeID, name string, isDir bool) {
		o++
		entries = append(entries, fuseutil.Dirent{
			Offset: fuseops.DirOffset(o), Inode: ino, Name: name,
			Type: memoryDirentType(isDir),
		})
	}
	switch {
	case op.Inode == memoryDirInode:
		for _, name := range memoryStaticDirentOrder {
			c := memoryStaticChildren[name]
			add(c.inode, name, c.isDir)
		}
	case op.Inode == memorySessionsInode || op.Inode == memoryDecisionsInode ||
		op.Inode == memoryKnowledgeInode:
		// Empty directories for now — populated dynamically.
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

func (fs *FileSystem) memoryAttr(inode fuseops.InodeID, isDir bool) fuseops.InodeAttributes {
	mode := os.FileMode(0444)
	var size uint64
	if isDir {
		mode, size = 0555|os.ModeDir, 4096
	} else {
		if inode == memoryNoteInode || inode == memorySearchInode || inode == memoryForgetInode {
			mode = 0644
		}
	}
	return fuseops.InodeAttributes{
		Size: size, Nlink: 1, Mode: mode,
		Atime: time.Now(), Mtime: time.Now(), Ctime: time.Now(),
		Uid: uint32(os.Getuid()), Gid: uint32(os.Getgid()),
	}
}

func (fs *FileSystem) memoryIsDir(inode fuseops.InodeID) bool {
	switch inode {
	case memoryDirInode, memorySessionsInode, memoryDecisionsInode, memoryPreferencesInode, memoryKnowledgeInode:
		return true
	}
	return false
}

func memoryDirentType(isDir bool) fuseutil.DirentType {
	if isDir {
		return fuseutil.DT_Directory
	}
	return fuseutil.DT_File
}

func truncateContent(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (fs *FileSystem) isMemoryStateWritable(inode fuseops.InodeID) bool {
	return inode == memoryNoteInode || inode == memorySearchInode || inode == memoryForgetInode
}
