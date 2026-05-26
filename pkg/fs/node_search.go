package fs

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agent-os/agent-os/pkg/index"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	searchDirName = "@search"

	searchInodeBase fuseops.InodeID = 0xFFFFF000
	searchDirInode                  = searchInodeBase + 0x000
	searchByTypeInode               = searchInodeBase + 0x001
	searchBySymbolInode             = searchInodeBase + 0x002
	searchByDependencyInode         = searchInodeBase + 0x003
	searchSimilarInode              = searchInodeBase + 0x004
	searchRecentInode               = searchInodeBase + 0x005

	searchTypeFunctionInode  = searchInodeBase + 0x010
	searchTypeMethodInode    = searchInodeBase + 0x011
	searchTypeClassInode     = searchInodeBase + 0x012
	searchTypeInterfaceInode = searchInodeBase + 0x013
	searchTypeVariableInode  = searchInodeBase + 0x014
	searchTypeConstantInode  = searchInodeBase + 0x015

	dynSymbolBase fuseops.InodeID = 0xFFF00000
)

var (
	searchSubDirs = map[string]fuseops.InodeID{
		"by-type": searchByTypeInode, "by-symbol": searchBySymbolInode,
		"by-dependency": searchByDependencyInode,
		"similar-to":    searchSimilarInode,
		"recent":        searchRecentInode,
	}
	searchTypeDirs = map[string]fuseops.InodeID{
		"function": searchTypeFunctionInode, "method": searchTypeMethodInode,
		"class": searchTypeClassInode, "interface": searchTypeInterfaceInode,
		"variable": searchTypeVariableInode, "constant": searchTypeConstantInode,
	}
	searchKindMap = map[fuseops.InodeID]index.SymbolKind{
		searchTypeFunctionInode:  index.KindFunction,
		searchTypeMethodInode:    index.KindMethod,
		searchTypeClassInode:     index.KindClass,
		searchTypeInterfaceInode: index.KindInterface,
		searchTypeVariableInode:  index.KindVariable,
		searchTypeConstantInode:  index.KindConstant,
	}
)

func hashInode(name string) fuseops.InodeID {
	h := md5.Sum([]byte(name))
	return dynSymbolBase + fuseops.InodeID(binary.LittleEndian.Uint32(h[:4])&0xFFFFF)
}
func (fs *FileSystem) indexEngine() *index.Engine {
	fs.indexOnce.Do(func() {
		fs.indexEng = index.NewEngine(fs.sourceDir, index.NewGoAnalyzer())
	})
	return fs.indexEng
}
func (fs *FileSystem) searchDirDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{
		Offset: offset, Inode: searchDirInode,
		Name: searchDirName, Type: fuseutil.DT_Directory,
	}
}
func (fs *FileSystem) isSearchInode(inode fuseops.InodeID) bool {
	return (inode >= searchInodeBase && inode < searchInodeBase+0x100) || inode >= dynSymbolBase
}
func (fs *FileSystem) trySearchLookup(name string, parent fuseops.InodeID) (*fuseops.ChildInodeEntry, bool) {
	var ino fuseops.InodeID
	var isDir bool

	switch {
	case parent == fuseops.RootInodeID && name == searchDirName:
		ino, isDir = searchDirInode, true
	case parent == searchDirInode:
		if id, ok := searchSubDirs[name]; ok {
			ino, isDir = id, true
		} else {
			return nil, false
		}
	case parent == searchByTypeInode:
		if id, ok := searchTypeDirs[name]; ok {
			ino, isDir = id, true
		} else {
			return nil, false
		}
	case parent == searchBySymbolInode:
		result, err := fs.indexEngine().GetAnalysis()
		if err != nil {
			return nil, false
		}
		for _, sym := range result.Symbols {
			if sym.Name == name {
				ino = hashInode(sym.Name)
				break
			}
		}
		if ino == 0 {
			return nil, false
		}
	case parent >= searchTypeFunctionInode && parent <= searchTypeConstantInode:
		result, err := fs.indexEngine().GetAnalysis()
		if err != nil {
			return nil, false
		}
		kind := searchKindMap[parent]
		for _, sym := range result.Symbols {
			if sym.Name == name && sym.Kind == kind {
				ino = hashInode(sym.Name)
				break
			}
		}
		if ino == 0 {
			return nil, false
		}
	default:
		return nil, false
	}

	return &fuseops.ChildInodeEntry{
		Child:      ino,
		Generation: 1,
		Attributes: fs.searchFileAttr(ino, isDir),
		AttributesExpiration: time.Now().Add(time.Second),
		EntryExpiration:      time.Now().Add(time.Second),
	}, true
}
func (fs *FileSystem) trySearchGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isSearchInode(op.Inode) {
		return false
	}
	op.Attributes = fs.searchFileAttr(op.Inode, (op.Inode >= searchInodeBase && op.Inode < searchInodeBase+0x100))
	op.AttributesExpiration = time.Now().Add(time.Second)
	return true
}

func (fs *FileSystem) trySearchRead(op *fuseops.ReadFileOp) bool {
	if !fs.isSearchInode(op.Inode) || (op.Inode >= searchInodeBase && op.Inode < searchInodeBase+0x100) {
		return false
	}
	data, err := fs.searchContent(op.Inode)
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

func (fs *FileSystem) trySearchOpenDir(op *fuseops.OpenDirOp) bool {
	if !(op.Inode >= searchInodeBase && op.Inode < searchInodeBase+0x100) {
		return false
	}
	entries, err := fs.searchDirContents(op.Inode)
	if err != nil {
		return false
	}
	handle := fs.allocHandle()
	fs.handleMu.Lock()
	fs.dirs[handle] = &dirHandle{entries: entries}
	fs.handleMu.Unlock()
	op.Handle = handle
	return true
}

func (fs *FileSystem) searchFileAttr(inode fuseops.InodeID, isDir bool) fuseops.InodeAttributes {
	mode := os.FileMode(0444)
	var size uint64
	if isDir {
		mode = 0555 | os.ModeDir
		size = 4096
	} else if data, err := fs.searchContent(inode); err == nil {
		size = uint64(len(data))
	}
	return fuseops.InodeAttributes{
		Size:  size,
		Nlink: 1,
		Mode:  mode,
		Atime: time.Now(),
		Mtime: time.Now(),
		Ctime: time.Now(),
		Uid:   uint32(os.Getuid()),
		Gid:   uint32(os.Getgid()),
	}
}

func (fs *FileSystem) searchContent(inode fuseops.InodeID) ([]byte, error) {
	result, err := fs.indexEngine().GetAnalysis()
	if err != nil {
		return nil, err
	}
	var matches []index.Symbol
	for _, sym := range result.Symbols {
		if hashInode(sym.Name) == inode {
			matches = append(matches, sym)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("symbol not found")
	}
	var b strings.Builder
	for i, sym := range matches {
		if i > 0 {
			b.WriteString("---\n")
		}
		fmt.Fprintf(&b, "Symbol: %s\n", sym.Name)
		fmt.Fprintf(&b, "Kind: %s\n", sym.Kind)
		fmt.Fprintf(&b, "Package: %s\n", sym.Package)
		if sym.Signature != "" {
			fmt.Fprintf(&b, "Signature: %s\n", sym.Signature)
		}
		fmt.Fprintf(&b, "Defined at: %s:%d:%d\n", sym.Def.File, sym.Def.Line, sym.Def.Column)
		if len(sym.Refs) > 0 {
			b.WriteString("\nReferences:\n")
			for _, ref := range sym.Refs {
				fmt.Fprintf(&b, "  - %s:%d:%d\n", ref.File, ref.Line, ref.Column)
			}
		}
		if i < len(matches)-1 {
			b.WriteString("\n")
		}
	}
	return []byte(b.String()), nil
}

func (fs *FileSystem) searchDirContents(inode fuseops.InodeID) ([]fuseutil.Dirent, error) {
	switch inode {
	case searchDirInode:
		return []fuseutil.Dirent{
			{Offset: 1, Inode: searchByTypeInode, Name: "by-type", Type: fuseutil.DT_Directory},
			{Offset: 2, Inode: searchBySymbolInode, Name: "by-symbol", Type: fuseutil.DT_Directory},
			{Offset: 3, Inode: searchByDependencyInode, Name: "by-dependency", Type: fuseutil.DT_Directory},
			{Offset: 4, Inode: searchSimilarInode, Name: "similar-to", Type: fuseutil.DT_Directory},
			{Offset: 5, Inode: searchRecentInode, Name: "recent", Type: fuseutil.DT_Directory},
		}, nil
	case searchByTypeInode:
		return []fuseutil.Dirent{
			{Offset: 1, Inode: searchTypeFunctionInode, Name: "function", Type: fuseutil.DT_Directory},
			{Offset: 2, Inode: searchTypeMethodInode, Name: "method", Type: fuseutil.DT_Directory},
			{Offset: 3, Inode: searchTypeClassInode, Name: "class", Type: fuseutil.DT_Directory},
			{Offset: 4, Inode: searchTypeInterfaceInode, Name: "interface", Type: fuseutil.DT_Directory},
			{Offset: 5, Inode: searchTypeVariableInode, Name: "variable", Type: fuseutil.DT_Directory},
			{Offset: 6, Inode: searchTypeConstantInode, Name: "constant", Type: fuseutil.DT_Directory},
		}, nil
	case searchBySymbolInode:
		return fs.dynamicSymbolDirents("")
	case searchTypeFunctionInode, searchTypeMethodInode,
		searchTypeClassInode, searchTypeInterfaceInode,
		searchTypeVariableInode, searchTypeConstantInode:
		return fs.dynamicSymbolDirents(searchKindMap[inode])
	default:
		return []fuseutil.Dirent{}, nil
	}
}

func (fs *FileSystem) dynamicSymbolDirents(kind index.SymbolKind) ([]fuseutil.Dirent, error) {
	result, err := fs.indexEngine().GetAnalysis()
	if err != nil {
		return nil, err
	}
	var dirents []fuseutil.Dirent
	seen := make(map[string]bool)
	offset := 0
	for _, sym := range result.Symbols {
		if seen[sym.Name] {
			continue
		}
		if kind != "" && sym.Kind != kind {
			continue
		}
		seen[sym.Name] = true
		offset++
		dirents = append(dirents, fuseutil.Dirent{
			Offset: fuseops.DirOffset(offset),
			Inode:  hashInode(sym.Name),
			Name:   sym.Name,
			Type:   fuseutil.DT_File,
		})
	}
	return dirents, nil
}
