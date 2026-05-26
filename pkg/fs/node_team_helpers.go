package fs

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agent-os/agent-os/pkg/team"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

// --- directory listing ---

func (fs *FileSystem) tryTeamOpenDir(op *fuseops.OpenDirOp) bool {
	if !fs.isTeamInode(op.Inode) {
		return false
	}
	ts := fs.teamStore()
	var entries []fuseutil.Dirent
	o := 0
	add := func(ino fuseops.InodeID, name string, dir bool) {
		o++
		entries = append(entries, fuseutil.Dirent{
			Offset: fuseops.DirOffset(o), Inode: ino, Name: name,
			Type: teamDirentType(dir),
		})
	}

	switch {
	case op.Inode == teamDirInode:
		for _, name := range teamStaticDirentOrder {
			c := teamStaticChildren[name]
			add(c.inode, name, c.isDir)
		}
		for _, a := range ts.ListAgents() {
			add(teamAgentInode(a.Name), a.Name, true)
		}

	case op.Inode == teamBroadcastInode:
		for _, a := range ts.ListAgents() {
			for _, m := range ts.GetInbox(a.Name) {
				add(dynMsgBase|(hashInode(m.ID)&0xFFFFF), teamMsgFileName(m), false)
			}
		}

	case op.Inode == teamSharedInode:
		for _, ctx := range ts.ListContexts("") {
			add(dynCtxBase|(hashInode(ctx.ID)&0xFFFFF), primaryTag(ctx)+".md", false)
		}

	case isAgentDir(op.Inode):
		for _, s := range agentSubDirents(op.Inode) {
			add(s.inode, s.name, s.isDir)
		}

	case isAgentSub(op.Inode, agentSubInbox):
		an := fs.teamAgentNameFromInode(op.Inode & ^agentSubMask)
		for _, m := range ts.GetInbox(an) {
			add(dynMsgBase|(hashInode(m.ID)&0xFFFFF), teamMsgFileName(m), false)
		}

	case isAgentSub(op.Inode, agentSubOutbox):
		an := fs.teamAgentNameFromInode(op.Inode & ^agentSubMask)
		for _, m := range ts.GetOutbox(an) {
			add(dynMsgBase|(hashInode(m.ID)&0xFFFFF), teamMsgFileName(m), false)
		}

	case isAgentSub(op.Inode, agentSubWorkspace):
		// empty

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

func agentSubDirents(base fuseops.InodeID) []struct {
	name  string
	inode fuseops.InodeID
	isDir bool
} {
	return []struct {
		name  string
		inode fuseops.InodeID
		isDir bool
	}{
		{"inbox", base | agentSubInbox, true},
		{"outbox", base | agentSubOutbox, true},
		{"workspace", base | agentSubWorkspace, true},
		{"context.md", base | agentSubContext, false},
	}
}

// --- agent inode helpers ---

const agentSubMask fuseops.InodeID = 0xFFF

func teamAgentInode(name string) fuseops.InodeID {
	return dynAgentBase | (hashInode(name) & 0xFFFFF)
}

func (fs *FileSystem) teamAgentNameFromInode(inode fuseops.InodeID) string {
	for _, a := range fs.teamStore().ListAgents() {
		if teamAgentInode(a.Name) == inode {
			return a.Name
		}
	}
	return ""
}

func isAgentSub(inode, sub fuseops.InodeID) bool {
	return inode >= dynAgentBase && inode < dynAgentBase+0x100000 && (inode&agentSubMask) == sub
}

func isAgentDir(inode fuseops.InodeID) bool {
	return inode >= dynAgentBase && inode < dynAgentBase+0x100000 && (inode&agentSubMask) == 0
}

func isAgentDirInode(inode fuseops.InodeID) bool { return isAgentDir(inode) }

func isAgentInode(inode fuseops.InodeID) bool {
	return inode >= dynAgentBase && inode < dynAgentBase+0x100000
}

func isAgentInboxInode(inode fuseops.InodeID) bool {
	return isAgentSub(inode, agentSubInbox)
}

func isAgentOutboxInode(inode fuseops.InodeID) bool {
	return isAgentSub(inode, agentSubOutbox)
}

func isAgentWorkspaceInode(inode fuseops.InodeID) bool {
	return isAgentSub(inode, agentSubWorkspace)
}

func teamDirentType(isDir bool) fuseutil.DirentType {
	if isDir {
		return fuseutil.DT_Directory
	}
	return fuseutil.DT_File
}

// --- attributes ---

func (fs *FileSystem) teamAttr(inode fuseops.InodeID, isDir bool) fuseops.InodeAttributes {
	mode := os.FileMode(0444)
	var size uint64
	if isDir {
		mode, size = 0555|os.ModeDir, 4096
	} else {
		if inode == teamTopologyInode {
			mode = 0644
		}
		if s := fs.teamFileSize(inode); s > 0 {
			size = s
		}
	}
	return fuseops.InodeAttributes{
		Size: size, Nlink: 1, Mode: mode,
		Atime: time.Now(), Mtime: time.Now(), Ctime: time.Now(),
		Uid: uint32(os.Getuid()), Gid: uint32(os.Getgid()),
	}
}

func (fs *FileSystem) teamFileSize(inode fuseops.InodeID) uint64 {
	data, err := fs.teamFileData(inode)
	if err != nil || data == nil {
		return 0
	}
	return uint64(len(data))
}

func (fs *FileSystem) teamFileData(inode fuseops.InodeID) ([]byte, error) {
	ts := fs.teamStore()
	switch {
	case inode == teamRosterInode:
		return ts.RosterYAML()
	case inode == teamTopologyInode:
		return ts.TopologyYAML()
	case inode == teamStatusInode:
		return ts.StatusText()
	case inode >= dynCtxBase && inode < dynCtxBase+0x100000:
		return fs.teamContextContent(inode)
	case inode >= dynMsgBase && inode < dynMsgBase+0x100000:
		return fs.teamMsgContent(inode)
	case isAgentInode(inode) && (inode&agentSubMask) == agentSubContext:
		an := fs.teamAgentNameFromInode(inode & ^agentSubMask)
		return fs.teamAgentContextContent(an)
	}
	return nil, nil
}

func (fs *FileSystem) teamIsDir(inode fuseops.InodeID) bool {
	if inode == teamDirInode || inode == teamBroadcastInode || inode == teamSharedInode {
		return true
	}
	if inode >= teamInodeBase && inode <= teamSharedInode {
		return false
	}
	if isAgentDir(inode) || isAgentSub(inode, agentSubInbox) ||
		isAgentSub(inode, agentSubOutbox) || isAgentSub(inode, agentSubWorkspace) {
		return true
	}
	return false
}

// --- content generators ---

func teamMsgFileName(m *team.Message) string {
	id := m.ID
	if len(id) > 8 {
		id = id[:8]
	}
	return fmt.Sprintf("%s_%s_%s.md", m.Timestamp.Format("150405"), m.From, id)
}

func primaryTag(ctx *team.SharedContext) string {
	if len(ctx.Tags) > 0 {
		return ctx.Tags[0]
	}
	return "untitled"
}

func (fs *FileSystem) teamContextContent(inode fuseops.InodeID) ([]byte, error) {
	for _, ctx := range fs.teamStore().ListContexts("") {
		if (dynCtxBase | (hashInode(ctx.ID) & 0xFFFFF)) == inode {
			return []byte(fmt.Sprintf("# %s\n\n%s\n", primaryTag(ctx), ctx.Content)), nil
		}
	}
	return nil, fmt.Errorf("context not found for inode %d", inode)
}

func (fs *FileSystem) teamMsgContent(inode fuseops.InodeID) ([]byte, error) {
	ts := fs.teamStore()
	var found *team.Message
	search := func(msgs []*team.Message) *team.Message {
		for _, m := range msgs {
			if (dynMsgBase | (hashInode(m.ID) & 0xFFFFF)) == inode {
				return m
			}
		}
		return nil
	}
	for _, a := range ts.ListAgents() {
		if found = search(ts.GetInbox(a.Name)); found != nil {
			break
		}
		if found = search(ts.GetOutbox(a.Name)); found != nil {
			break
		}
	}
	if found == nil {
		found = search(ts.GetInbox("@all"))
	}
	if found == nil {
		return nil, fmt.Errorf("message not found for inode %d", inode)
	}

	if !found.Read {
		_ = ts.MarkRead(found.To, found.ID)
		found.Read = true
	}

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\nTo: %s\n", found.From, found.To)
	if found.Subject != "" {
		fmt.Fprintf(&b, "Subject: %s\n", found.Subject)
	}
	fmt.Fprintf(&b, "Time: %s\nRead: %v\n\n%s\n",
		found.Timestamp.Format(time.RFC3339), found.Read, found.Body)
	return []byte(b.String()), nil
}

func (fs *FileSystem) teamAgentContextContent(agentName string) ([]byte, error) {
	list := fs.teamStore().ListContexts(agentName)
	if len(list) == 0 {
		return []byte(fmt.Sprintf("# Context for %s\n\n(empty)\n", agentName)), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Context for %s\n\n", agentName)
	for _, ctx := range list {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", primaryTag(ctx), ctx.Content)
	}
	return []byte(b.String()), nil
}


