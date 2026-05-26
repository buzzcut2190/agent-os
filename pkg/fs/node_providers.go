package fs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agent-os/agent-os/pkg/provider"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	providersDirName = "@providers"

	providersInodeBase       fuseops.InodeID = 0xFFFFF600
	providersDirInode                        = providersInodeBase + 0x000
	providersListDirInode                    = providersInodeBase + 0x001
	providersAgentDirInode                   = providersInodeBase + 0x002
	providersKeysDirInode                    = providersInodeBase + 0x003
	providersTestInode                       = providersInodeBase + 0x004
	providersSwitchInode                     = providersInodeBase + 0x005
	providersModelsInode                     = providersInodeBase + 0x006
	providersActiveInode                     = providersInodeBase + 0x007
	providersAgentDefaultInode               = providersInodeBase + 0x008
	providersAgentStatusInode                = providersInodeBase + 0x009
	providersLogsInode                       = providersInodeBase + 0x00A

	dynProviderBase fuseops.InodeID = 0xFF900000
)

var providersStaticChildren = map[string]struct {
	inode fuseops.InodeID
	isDir bool
}{
	"list":   {providersListDirInode, true},
	"agent":  {providersAgentDirInode, true},
	"keys":   {providersKeysDirInode, true},
	"test":   {providersTestInode, false},
	"switch": {providersSwitchInode, false},
	"models": {providersModelsInode, false},
	"active": {providersActiveInode, false},
	"logs":   {providersLogsInode, false},
}

var providersStaticDirentOrder = []string{"list", "agent", "keys", "test", "switch", "models", "active", "logs"}

// --- Global provider state ---
var (
	globalRegistry *provider.Registry
	globalRouter   *provider.Router
	globalKeyStore *provider.KeyStore
)

// SetProviderState injects the provider components for FUSE access.
func SetProviderState(reg *provider.Registry, router *provider.Router, ks *provider.KeyStore) {
	globalRegistry = reg
	globalRouter = router
	globalKeyStore = ks
}

func (fs *FileSystem) isProvidersInode(inode fuseops.InodeID) bool {
	return (inode >= providersInodeBase && inode <= providersLogsInode) ||
		(inode >= dynProviderBase && inode < dynProviderBase+0x100000)
}

func providerKeyInode(providerName string) fuseops.InodeID {
	return dynProviderBase | (hashInode(providerName+"_key") & 0xFFFFF)
}

func providerListInode(providerName string) fuseops.InodeID {
	return dynProviderBase | (hashInode(providerName+"_list") & 0xFFFFF)
}

func (fs *FileSystem) providersDirDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{
		Offset: offset, Inode: providersDirInode,
		Name: providersDirName, Type: fuseutil.DT_Directory,
	}
}

func (fs *FileSystem) tryProvidersLookup(name string, parent fuseops.InodeID) (*fuseops.ChildInodeEntry, bool) {
	if globalRegistry == nil {
		return nil, false
	}

	switch {
	case parent == fuseops.RootInodeID && name == providersDirName:
		return fs.providersChildEntry(providersDirInode, true), true

	case parent == providersDirInode:
		if c, ok := providersStaticChildren[name]; ok {
			return fs.providersChildEntry(c.inode, c.isDir), true
		}
		return nil, false

	case parent == providersListDirInode:
		if _, ok := globalRegistry.Get(name); ok {
			return fs.providersChildEntry(providerListInode(name), false), true
		}
		return nil, false

	case parent == providersAgentDirInode:
		switch name {
		case "default":
			return fs.providersChildEntry(providersAgentDefaultInode, false), true
		case "status":
			return fs.providersChildEntry(providersAgentStatusInode, false), true
		}
		return nil, false

	case parent == providersKeysDirInode:
		if _, ok := globalKeyStore.Get(name); ok {
			return fs.providersChildEntry(providerKeyInode(name), false), true
		}
		return nil, false

	default:
		return nil, false
	}
}

func (fs *FileSystem) providersChildEntry(ino fuseops.InodeID, isDir bool) *fuseops.ChildInodeEntry {
	return &fuseops.ChildInodeEntry{
		Child:                ino,
		Generation:           1,
		Attributes:           fs.providersAttr(ino, isDir),
		AttributesExpiration: time.Now().Add(5 * time.Second),
		EntryExpiration:      time.Now().Add(5 * time.Second),
	}
}

func (fs *FileSystem) tryProvidersGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isProvidersInode(op.Inode) {
		return false
	}
	isDir := fs.providersIsDir(op.Inode)
	op.Attributes = fs.providersAttr(op.Inode, isDir)
	op.AttributesExpiration = time.Now().Add(5 * time.Second)
	return true
}

func (fs *FileSystem) tryProvidersRead(op *fuseops.ReadFileOp) bool {
	if !fs.isProvidersInode(op.Inode) {
		return false
	}
	var data []byte

	switch {
	case op.Inode == providersTestInode:
		data = fs.providersTestContent()
	case op.Inode == providersSwitchInode:
		data = []byte(fmt.Sprintf("default: %s\n", globalRouter.GetDefault()))
	case op.Inode == providersModelsInode:
		data = fs.providersModelsContent()
	case op.Inode == providersActiveInode:
		data = []byte(fmt.Sprintf("default: %s\n", globalRouter.GetDefault()))
	case op.Inode == providersAgentDefaultInode:
		data = []byte(globalRouter.GetDefault() + "\n")
	case op.Inode == providersAgentStatusInode:
		data = fs.providersStatusContent()
	case op.Inode == providersLogsInode:
		data = []byte("(API call logs — last 50)\n")
	case op.Inode >= dynProviderBase && op.Inode < dynProviderBase+0x100000:
		// Dynamic key inode.
		providerName := fs.providerNameFromKeyInode(op.Inode)
		if providerName != "" {
			key, ok := globalKeyStore.Get(providerName)
			if ok {
				data = []byte(provider.Mask(key) + "\n")
			} else {
				data = []byte("(not set)\n")
			}
		} else {
			// Dynamic list inode.
			data = fs.providersListDetail(op.Inode)
		}
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

func (fs *FileSystem) tryProvidersWrite(op *fuseops.WriteFileOp) bool {
	if !fs.isProvidersInode(op.Inode) {
		return false
	}

	switch {
	case op.Inode == providersAgentDefaultInode || op.Inode == providersSwitchInode:
		if op.Offset != 0 {
			return true
		}
		name := strings.TrimSpace(string(op.Data))
		globalRouter.SetDefault(name)
		return true

	case op.Inode >= dynProviderBase && op.Inode < dynProviderBase+0x100000:
		providerName := fs.providerNameFromKeyInode(op.Inode)
		if providerName != "" && op.Offset == 0 {
			key := strings.TrimSpace(string(op.Data))
			globalKeyStore.Set(providerName, key)
			return true
		}
		return false

	default:
		return false
	}
}

func (fs *FileSystem) tryProvidersOpenDir(op *fuseops.OpenDirOp) bool {
	if !fs.isProvidersInode(op.Inode) {
		return false
	}
	if !fs.providersIsDir(op.Inode) {
		return false
	}
	entries := make([]fuseutil.Dirent, 0)
	o := 0
	add := func(ino fuseops.InodeID, name string, isDir bool) {
		o++
		entries = append(entries, fuseutil.Dirent{
			Offset: fuseops.DirOffset(o), Inode: ino, Name: name,
			Type: providersDirentType(isDir),
		})
	}

	switch {
	case op.Inode == providersDirInode:
		for _, name := range providersStaticDirentOrder {
			c := providersStaticChildren[name]
			add(c.inode, name, c.isDir)
		}

	case op.Inode == providersListDirInode:
		for _, p := range globalRegistry.All() {
			add(providerListInode(p.Name()), p.Name(), false)
		}

	case op.Inode == providersAgentDirInode:
		add(providersAgentDefaultInode, "default", false)
		add(providersAgentStatusInode, "status", false)

	case op.Inode == providersKeysDirInode:
		for _, info := range globalKeyStore.List() {
			add(providerKeyInode(info.Provider), info.Provider, false)
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

// --- helpers ---

func (fs *FileSystem) providersAttr(inode fuseops.InodeID, isDir bool) fuseops.InodeAttributes {
	mode := os.FileMode(0444)
	var size uint64
	if isDir {
		mode, size = 0555|os.ModeDir, 4096
	} else {
		// Writable inodes.
		if inode == providersAgentDefaultInode || inode == providersSwitchInode ||
			(inode >= dynProviderBase && inode < dynProviderBase+0x100000) {
			mode = 0644
		}
		if s := fs.providersFileSize(inode); s > 0 {
			size = s
		}
	}
	return fuseops.InodeAttributes{
		Size: size, Nlink: 1, Mode: mode,
		Atime: time.Now(), Mtime: time.Now(), Ctime: time.Now(),
		Uid: uint32(os.Getuid()), Gid: uint32(os.Getgid()),
	}
}

func (fs *FileSystem) providersFileSize(inode fuseops.InodeID) uint64 {
	switch {
	case inode == providersAgentDefaultInode:
		return uint64(len(globalRouter.GetDefault()) + 1)
	case inode == providersSwitchInode:
		return uint64(len(globalRouter.GetDefault()) + 20)
	case inode == providersTestInode:
		return uint64(len(fs.providersTestContent()))
	case inode == providersModelsInode:
		return uint64(len(fs.providersModelsContent()))
	case inode == providersActiveInode:
		return uint64(len(globalRouter.GetDefault()) + 20)
	case inode == providersAgentStatusInode:
		return uint64(len(fs.providersStatusContent()))
	}
	return 0
}

func (fs *FileSystem) providersIsDir(inode fuseops.InodeID) bool {
	return inode == providersDirInode ||
		inode == providersListDirInode ||
		inode == providersAgentDirInode ||
		inode == providersKeysDirInode
}

func providersDirentType(isDir bool) fuseutil.DirentType {
	if isDir {
		return fuseutil.DT_Directory
	}
	return fuseutil.DT_File
}

func (fs *FileSystem) providersTestContent() []byte {
	var b strings.Builder
	b.WriteString("Provider Health Check\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")
	for _, p := range globalRegistry.All() {
		status := "UNHEALTHY"
		if err := p.Ping(context.Background()); err == nil {
			status = "HEALTHY"
		}
		fmt.Fprintf(&b, "%-20s %s\n", p.Name(), status)
	}
	return []byte(b.String())
}

func (fs *FileSystem) providersModelsContent() []byte {
	var b strings.Builder
	for _, m := range globalRouter.GetAllModels() {
		fmt.Fprintf(&b, "%s: %s\n", m.Provider, m.ID)
	}
	return []byte(b.String())
}

func (fs *FileSystem) providersStatusContent() []byte {
	def := globalRouter.GetDefault()
	return []byte(fmt.Sprintf("当前: %s | 状态: 就绪\n", def))
}

func (fs *FileSystem) providersListDetail(inode fuseops.InodeID) []byte {
	for _, p := range globalRegistry.All() {
		if providerListInode(p.Name()) == inode {
			var b strings.Builder
			fmt.Fprintf(&b, "Name: %s\n", p.Name())
			fmt.Fprintf(&b, "Models: ")
			for i, m := range p.Models() {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(m.ID)
			}
			b.WriteString("\n")
			return []byte(b.String())
		}
	}
	return nil
}

func (fs *FileSystem) providerNameFromKeyInode(inode fuseops.InodeID) string {
	for _, info := range globalKeyStore.List() {
		if providerKeyInode(info.Provider) == inode {
			return info.Provider
		}
	}
	return ""
}

func (fs *FileSystem) isProvidersStateWritable(inode fuseops.InodeID) bool {
	return inode == providersAgentDefaultInode ||
		inode == providersSwitchInode ||
		(inode >= dynProviderBase && inode < dynProviderBase+0x100000)
}
