package fs

import (
	"fmt"
	"strings"
	"time"

	"github.com/agent-os/agent-os/pkg/team"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	teamDirName = "@team"

	teamInodeBase      fuseops.InodeID = 0xFFFFF300
	teamDirInode                       = teamInodeBase + 0x000
	teamRosterInode                    = teamInodeBase + 0x001
	teamTopologyInode                  = teamInodeBase + 0x002
	teamStatusInode                    = teamInodeBase + 0x003
	teamBroadcastInode                 = teamInodeBase + 0x004
	teamSharedInode                    = teamInodeBase + 0x005

	dynAgentBase fuseops.InodeID = 0xFFE00000
	dynMsgBase   fuseops.InodeID = 0xFFC00000
	dynCtxBase   fuseops.InodeID = 0xFFB00000

	agentSubInbox     fuseops.InodeID = 0x010
	agentSubOutbox    fuseops.InodeID = 0x020
	agentSubWorkspace fuseops.InodeID = 0x030
	agentSubContext   fuseops.InodeID = 0x040
)

var teamStaticChildren = map[string]struct {
	inode fuseops.InodeID
	isDir bool
}{
	"roster":    {teamRosterInode, false},
	"topology":  {teamTopologyInode, false},
	"status":    {teamStatusInode, false},
	"broadcast": {teamBroadcastInode, true},
	"shared":    {teamSharedInode, true},
}

var teamStaticDirentOrder = []string{"roster", "topology", "status", "broadcast", "shared"}

func (fs *FileSystem) teamStore() *team.TeamStore {
	fs.teamOnce.Do(func() {
		fs.teamEng = team.NewTeamStore()
	})
	return fs.teamEng
}

func (fs *FileSystem) isTeamInode(inode fuseops.InodeID) bool {
	return (inode >= teamInodeBase && inode <= teamSharedInode) ||
		(inode >= dynAgentBase && inode < dynAgentBase+0x100000) ||
		(inode >= dynMsgBase && inode < dynMsgBase+0x100000) ||
		(inode >= dynCtxBase && inode < dynCtxBase+0x100000)
}

func (fs *FileSystem) teamDirDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{
		Offset: offset, Inode: teamDirInode,
		Name: teamDirName, Type: fuseutil.DT_Directory,
	}
}

func (fs *FileSystem) tryTeamLookup(name string, parent fuseops.InodeID) (*fuseops.ChildInodeEntry, bool) {
	ts := fs.teamStore()
	var ino fuseops.InodeID
	var isDir bool

	switch {
	case parent == fuseops.RootInodeID && name == teamDirName:
		ino, isDir = teamDirInode, true

	case parent == teamDirInode:
		if c, ok := teamStaticChildren[name]; ok {
			ino, isDir = c.inode, c.isDir
		} else if _, ok := ts.GetAgent(name); ok {
			ino, isDir = teamAgentInode(name), true
		} else {
			return nil, false
		}

	case parent == teamSharedInode:
		for _, ctx := range ts.ListContexts("") {
			if name == primaryTag(ctx)+".md" {
				ino = dynCtxBase | (hashInode(ctx.ID) & 0xFFFFF)
				break
			}
		}
		if ino == 0 {
			return nil, false
		}

	case parent == teamBroadcastInode:
		ino = dynMsgBase | (hashInode(name) & 0xFFFFF)

	case isAgentDirInode(parent):
		_ = fs.teamAgentNameFromInode(parent)
		switch {
		case name == "inbox":
			ino, isDir = parent|agentSubInbox, true
		case name == "outbox":
			ino, isDir = parent|agentSubOutbox, true
		case name == "workspace":
			ino, isDir = parent|agentSubWorkspace, true
		case name == "context.md":
			ino = parent | agentSubContext
		default:
			return nil, false
		}

	case isAgentInboxInode(parent):
		an := fs.teamAgentNameFromInode(parent & ^agentSubInbox)
		for _, m := range ts.GetInbox(an) {
			if name == teamMsgFileName(m) {
				ino = dynMsgBase | (hashInode(m.ID) & 0xFFFFF)
				break
			}
		}
		if ino == 0 {
			return nil, false
		}

	case isAgentOutboxInode(parent):
		an := fs.teamAgentNameFromInode(parent & ^agentSubOutbox)
		for _, m := range ts.GetOutbox(an) {
			if name == teamMsgFileName(m) {
				ino = dynMsgBase | (hashInode(m.ID) & 0xFFFFF)
				break
			}
		}
		if ino == 0 {
			return nil, false
		}

	case isAgentWorkspaceInode(parent):
		return nil, false

	default:
		return nil, false
	}

	return &fuseops.ChildInodeEntry{
		Child:                 ino,
		Generation:            1,
		Attributes:            fs.teamAttr(ino, isDir),
		AttributesExpiration:  time.Now().Add(time.Second),
		EntryExpiration:       time.Now().Add(time.Second),
	}, true
}

func (fs *FileSystem) tryTeamGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isTeamInode(op.Inode) {
		return false
	}
	isDir := fs.teamIsDir(op.Inode)
	op.Attributes = fs.teamAttr(op.Inode, isDir)
	op.AttributesExpiration = time.Now().Add(time.Second)
	return true
}

func (fs *FileSystem) tryTeamRead(op *fuseops.ReadFileOp) bool {
	if !fs.isTeamInode(op.Inode) {
		return false
	}
	ts := fs.teamStore()
	var data []byte
	var err error

	switch {
	case op.Inode == teamRosterInode:
		data, err = ts.RosterYAML()
	case op.Inode == teamTopologyInode:
		data, err = ts.TopologyYAML()
	case op.Inode == teamStatusInode:
		data, err = ts.StatusText()
	case op.Inode >= dynCtxBase && op.Inode < dynCtxBase+0x100000:
		data, err = fs.teamContextContent(op.Inode)
	case op.Inode >= dynMsgBase && op.Inode < dynMsgBase+0x100000:
		data, err = fs.teamMsgContent(op.Inode)
	case isAgentInode(op.Inode) && (op.Inode&0xFFF) == agentSubContext:
		an := fs.teamAgentNameFromInode(op.Inode & ^agentSubContext)
		data, err = fs.teamAgentContextContent(an)
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

func (fs *FileSystem) tryTeamWrite(op *fuseops.WriteFileOp) bool {
	if !fs.isTeamInode(op.Inode) {
		return false
	}
	ts := fs.teamStore()

	switch {
	case op.Inode == teamTopologyInode:
		if op.Offset != 0 {
			return false
		}
		return ts.ParseTopologyYAML(op.Data) == nil

	case op.Inode == teamBroadcastInode, op.Inode >= dynMsgBase && op.Inode < dynMsgBase+0x100000:
		return false

	case isAgentOutboxInode(op.Inode):
		an := fs.teamAgentNameFromInode(op.Inode & ^agentSubOutbox)
		content := string(op.Data)
		if strings.TrimSpace(content) == "" {
			return false
		}
		to, subject, body := parseMsgWrite(content)
		if to == "@all" {
			_ = ts.Broadcast(an, subject, body)
		} else {
			_ = ts.SendMessage(team.Message{
				From: an, To: to, Subject: subject, Body: body,
			})
		}
		return true

	case isAgentWorkspaceInode(op.Inode):
		return false

	default:
		return false
	}
}

func parseMsgWrite(content string) (to, subject, body string) {
	lines := strings.Split(content, "\n")
	bs := 0
	for i, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(lower, "to:"):
			to = strings.TrimSpace(line[3:])
			bs = i + 1
		case strings.HasPrefix(lower, "subject:"):
			subject = strings.TrimSpace(line[8:])
			bs = i + 1
		case strings.HasPrefix(lower, "from:"):
			bs = i + 1
		default:
			goto done
		}
	}
done:
	if bs < len(lines) {
		body = strings.TrimSpace(strings.Join(lines[bs:], "\n"))
	}
	if body == "" {
		body = content
	}
	return
}


