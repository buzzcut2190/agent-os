package fs

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agent-os/agent-os/pkg/skill"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	skillsDirName = "@skills"

	skillsInodeBase      fuseops.InodeID = 0xFFFFF500
	skillsDirInode                       = skillsInodeBase + 0x000
	skillsActiveDirInode                 = skillsInodeBase + 0x001
	skillsDiscoverDirInode               = skillsInodeBase + 0x002
	skillsNewDirInode                    = skillsInodeBase + 0x003

	dynSkillBase fuseops.InodeID = 0xFFA00000

	skillSubMetadata   fuseops.InodeID = 0x010
	skillSubState      fuseops.InodeID = 0x020
	skillSubActivate   fuseops.InodeID = 0x030
	skillSubDeactivate fuseops.InodeID = 0x040
	skillSubContext    fuseops.InodeID = 0x050
	skillSubPrompt     fuseops.InodeID = 0x060
	skillSubMask       fuseops.InodeID = 0xFFF
)

var skillSubFiles = []struct {
	name  string
	sub   fuseops.InodeID
	isDir bool
}{
	{"metadata", skillSubMetadata, false},
	{"state", skillSubState, false},
	{"activate", skillSubActivate, false},
	{"deactivate", skillSubDeactivate, false},
	{"context.md", skillSubContext, false},
	{"prompt.md", skillSubPrompt, false},
}

func (fs *FileSystem) skillEngine() *skill.Engine {
	fs.skillOnce.Do(func() {
		fs.skillEng = skill.NewEngine("", "")
		fs.skillEng.LoadAll()
	})
	return fs.skillEng
}

func (fs *FileSystem) isSkillsInode(inode fuseops.InodeID) bool {
	return (inode >= skillsInodeBase && inode <= skillsNewDirInode) ||
		(inode >= dynSkillBase && inode < dynSkillBase+0x100000)
}

func skillInodeForName(name string) fuseops.InodeID {
	return dynSkillBase | (hashInode(name) & 0xFFFFF)
}

func (fs *FileSystem) skillNameFromInode(inode fuseops.InodeID) string {
	base := inode & ^skillSubMask
	se := fs.skillEngine()
	for _, s := range se.List() {
		if skillInodeForName(s.Name) == base {
			return s.Name
		}
	}
	return ""
}

func (fs *FileSystem) skillsDirDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{
		Offset: offset, Inode: skillsDirInode,
		Name: skillsDirName, Type: fuseutil.DT_Directory,
	}
}

func (fs *FileSystem) trySkillsLookup(name string, parent fuseops.InodeID) (*fuseops.ChildInodeEntry, bool) {
	se := fs.skillEngine()

	switch {
	case parent == fuseops.RootInodeID && name == skillsDirName:
		ino, isDir := skillsDirInode, true
		return fs.skillsChildEntry(ino, isDir), true

	case parent == skillsDirInode:
		switch name {
		case "active":
			return fs.skillsChildEntry(skillsActiveDirInode, true), true
		case "discover":
			return fs.skillsChildEntry(skillsDiscoverDirInode, true), true
		case "new":
			return fs.skillsChildEntry(skillsNewDirInode, true), true
		}
		if _, err := se.Get(name); err == nil {
			return fs.skillsChildEntry(skillInodeForName(name), true), true
		}
		return nil, false

	case parent == skillsActiveDirInode:
		for _, s := range se.Active() {
			if name == s.Name {
				return fs.skillsChildEntry(skillInodeForName(s.Name), true), true
			}
		}
		return nil, false

	case parent == skillsDiscoverDirInode:
		return nil, false

	case parent == skillsNewDirInode:
		return nil, false

	case isSkillDirInode(parent):
		_ = fs.skillNameFromInode(parent)
		for _, sf := range skillSubFiles {
			if name == sf.name {
				return fs.skillsChildEntry(parent|sf.sub, sf.isDir), true
			}
		}
		return nil, false

	default:
		return nil, false
	}
}

func (fs *FileSystem) skillsChildEntry(ino fuseops.InodeID, isDir bool) *fuseops.ChildInodeEntry {
	return &fuseops.ChildInodeEntry{
		Child:                ino,
		Generation:           1,
		Attributes:           fs.skillsAttr(ino, isDir),
		AttributesExpiration: time.Now().Add(5 * time.Second),
		EntryExpiration:      time.Now().Add(5 * time.Second),
	}
}

func (fs *FileSystem) trySkillsGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isSkillsInode(op.Inode) {
		return false
	}
	isDir := fs.skillsIsDir(op.Inode)
	op.Attributes = fs.skillsAttr(op.Inode, isDir)
	op.AttributesExpiration = time.Now().Add(5 * time.Second)
	return true
}

func (fs *FileSystem) trySkillsRead(op *fuseops.ReadFileOp) bool {
	if !fs.isSkillsInode(op.Inode) {
		return false
	}
	se := fs.skillEngine()
	var data []byte
	var err error

	switch {
	case op.Inode == skillsDiscoverDirInode:
		data, err = fs.skillsDiscoverContent()
	case op.Inode == skillsNewDirInode:
		data = []byte("# Create a new skill\n\nWrite a YAML or JSON definition to this file to create a new skill.\n\nExample:\n```yaml\nname: my-skill\ndescription: My custom skill\nversion: 1.0.0\nauthor: me\ntags: [custom]\n```\n")
	case isSkillSub(op.Inode, skillSubMetadata):
		data, err = se.GetMetadata(fs.skillNameFromInode(op.Inode))
	case isSkillSub(op.Inode, skillSubActivate):
		name := fs.skillNameFromInode(op.Inode)
		if err = se.Activate(name); err == nil {
			data = []byte(fmt.Sprintf("activated %s\n", name))
		}
	case isSkillSub(op.Inode, skillSubDeactivate):
		name := fs.skillNameFromInode(op.Inode)
		if err = se.Deactivate(name); err == nil {
			data = []byte(fmt.Sprintf("deactivated %s\n", name))
		}
	case isSkillSub(op.Inode, skillSubContext):
		var ctx string
		ctx, err = se.GetContext(fs.skillNameFromInode(op.Inode))
		if err == nil {
			data = []byte(ctx)
		}
	case isSkillSub(op.Inode, skillSubPrompt):
		var prompt string
		prompt, err = se.GetPrompt(fs.skillNameFromInode(op.Inode), false)
		if err == nil {
			data = []byte(prompt)
		}
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

func (fs *FileSystem) trySkillsWrite(op *fuseops.WriteFileOp) bool {
	if !fs.isSkillsInode(op.Inode) {
		return false
	}
	se := fs.skillEngine()

	switch {
	case isSkillSub(op.Inode, skillSubState):
		if op.Offset != 0 {
			return true
		}
		name := fs.skillNameFromInode(op.Inode)
		val := strings.TrimSpace(string(op.Data))
		state := skill.ParseState(val)
		switch state {
		case skill.SkillActive:
			se.Activate(name)
			se.SaveState()
			return true
		case skill.SkillInactive:
			se.Deactivate(name)
			se.SaveState()
			return true
		default:
			return false
		}

	case op.Inode == skillsNewDirInode:
		return true

	default:
		return false
	}
}

func (fs *FileSystem) trySkillsOpenDir(op *fuseops.OpenDirOp) bool {
	if !fs.isSkillsInode(op.Inode) {
		return false
	}
	if !fs.skillsIsDir(op.Inode) {
		return false
	}
	se := fs.skillEngine()
	entries := make([]fuseutil.Dirent, 0)
	o := 0
	add := func(ino fuseops.InodeID, name string, isDir bool) {
		o++
		entries = append(entries, fuseutil.Dirent{
			Offset: fuseops.DirOffset(o), Inode: ino, Name: name,
			Type: skillsDirentType(isDir),
		})
	}

	switch {
	case op.Inode == skillsDirInode:
		add(skillsActiveDirInode, "active", true)
		add(skillsDiscoverDirInode, "discover", true)
		add(skillsNewDirInode, "new", true)
		skills := se.List()
		sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
		for _, s := range skills {
			add(skillInodeForName(s.Name), s.Name, true)
		}

	case op.Inode == skillsActiveDirInode:
		for _, s := range se.Active() {
			add(skillInodeForName(s.Name), s.Name, true)
		}

	case op.Inode == skillsDiscoverDirInode:
		return false

	case op.Inode == skillsNewDirInode:
		return false

	case isSkillDirInode(op.Inode):
		for _, sf := range skillSubFiles {
			add(op.Inode|sf.sub, sf.name, sf.isDir)
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

func (fs *FileSystem) skillsAttr(inode fuseops.InodeID, isDir bool) fuseops.InodeAttributes {
	mode := os.FileMode(0444)
	var size uint64
	if isDir {
		mode, size = 0555|os.ModeDir, 4096
	} else {
		if isSkillSub(inode, skillSubState) || inode == skillsNewDirInode {
			mode = 0644
		}
		if s := fs.skillsFileSize(inode); s > 0 {
			size = s
		}
	}
	return fuseops.InodeAttributes{
		Size: size, Nlink: 1, Mode: mode,
		Atime: time.Now(), Mtime: time.Now(), Ctime: time.Now(),
		Uid: uint32(os.Getuid()), Gid: uint32(os.Getgid()),
	}
}

func (fs *FileSystem) skillsFileSize(inode fuseops.InodeID) uint64 {
	se := fs.skillEngine()
	switch {
	case isSkillSub(inode, skillSubMetadata):
		if d, err := se.GetMetadata(fs.skillNameFromInode(inode)); err == nil {
			return uint64(len(d))
		}
	case isSkillSub(inode, skillSubState):
		name := fs.skillNameFromInode(inode)
		if s, err := se.Get(name); err == nil {
			return uint64(len(s.State.String()))
		}
	case isSkillSub(inode, skillSubActivate), isSkillSub(inode, skillSubDeactivate):
		return 0
	case isSkillSub(inode, skillSubContext):
		if d, err := se.GetContext(fs.skillNameFromInode(inode)); err == nil {
			return uint64(len(d))
		}
	case isSkillSub(inode, skillSubPrompt):
		if d, err := se.GetPrompt(fs.skillNameFromInode(inode), false); err == nil {
			return uint64(len(d))
		}
	case inode == skillsDiscoverDirInode:
		if d, err := fs.skillsDiscoverContent(); err == nil {
			return uint64(len(d))
		}
	}
	return 0
}

func (fs *FileSystem) skillsIsDir(inode fuseops.InodeID) bool {
	switch {
	case inode == skillsDirInode:
		return true
	case inode == skillsActiveDirInode:
		return true
	case inode == skillsDiscoverDirInode:
		return false
	case inode == skillsNewDirInode:
		return false
	case isSkillDirInode(inode):
		return true
	case isSkillSubInode(inode):
		return false
	}
	return false
}

func isSkillDirInode(inode fuseops.InodeID) bool {
	return inode >= dynSkillBase && inode < dynSkillBase+0x100000 && (inode&skillSubMask) == 0
}

func isSkillSubInode(inode fuseops.InodeID) bool {
	return inode >= dynSkillBase && inode < dynSkillBase+0x100000 && (inode&skillSubMask) != 0
}

func isSkillSub(inode, sub fuseops.InodeID) bool {
	return inode >= dynSkillBase && inode < dynSkillBase+0x100000 && (inode&skillSubMask) == sub
}

func skillsDirentType(isDir bool) fuseutil.DirentType {
	if isDir {
		return fuseutil.DT_Directory
	}
	return fuseutil.DT_File
}

func (fs *FileSystem) skillsDiscoverContent() ([]byte, error) {
	se := fs.skillEngine()
	var discovered []string
	for _, dir := range []string{userSkillDirPath()} {
		if d, err := se.Discover(dir); err == nil && len(d) > 0 {
			discovered = append(discovered, d...)
		}
	}
	if len(discovered) == 0 {
		return []byte("No new skills discovered.\n"), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Discovered %d new skill(s):\n", len(discovered))
	for _, name := range discovered {
		fmt.Fprintf(&b, "  - %s\n", name)
	}
	return []byte(b.String()), nil
}

func userSkillDirPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return home + "/.config/agentfs/skills"
}

func (fs *FileSystem) isSkillsStateWritable(inode fuseops.InodeID) bool {
	return isSkillSub(inode, skillSubState) || inode == skillsNewDirInode
}
