package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agent-os/agent-os/pkg/kernel"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	kernelDirName = "@kernel"

	kernelInodeBase         fuseops.InodeID = 0xFFFFF900
	kernelDirInode                          = kernelInodeBase + 0x000
	kernelAgentsDirInode                    = kernelInodeBase + 0x001
	kernelTasksDirInode                     = kernelInodeBase + 0x002
	kernelResourcesDirInode                 = kernelInodeBase + 0x003
	kernelChannelsDirInode                  = kernelInodeBase + 0x004
	kernelConfigInode                       = kernelInodeBase + 0x005
	kernelRestartInode                      = kernelInodeBase + 0x006
	kernelAgentSpawnInode                   = kernelInodeBase + 0x007
	kernelTasksSubmitInode                  = kernelInodeBase + 0x008
	kernelResourcesUsageInode               = kernelInodeBase + 0x009
	kernelResourcesLimitsInode              = kernelInodeBase + 0x00A
	kernelAgentSummaryInode                 = kernelInodeBase + 0x00B

	kernelAgentInodeBase fuseops.InodeID = 0xFF800000
)

var (
	globalKernelLM *kernel.LifecycleManager
	globalKernelS  *kernel.Scheduler
	globalKernelRM *kernel.ResourceManager
	globalKernelIPC *kernel.IPC
)

func SetKernelState(lm *kernel.LifecycleManager, s *kernel.Scheduler, rm *kernel.ResourceManager, ipc *kernel.IPC) {
	globalKernelLM = lm
	globalKernelS = s
	globalKernelRM = rm
	globalKernelIPC = ipc
}

func (fs *FileSystem) isKernelInode(inode fuseops.InodeID) bool {
	return (inode >= kernelInodeBase && inode <= kernelAgentSummaryInode) ||
		(inode >= kernelAgentInodeBase && inode < kernelAgentInodeBase+0x100000)
}

func kernelAgentInode(id kernel.AgentID) fuseops.InodeID {
	return kernelAgentInodeBase | (hashInode(string(id)) & 0xFFFFF)
}

func (fs *FileSystem) kernelDirDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{Offset: offset, Inode: kernelDirInode, Name: kernelDirName, Type: fuseutil.DT_Directory}
}

func (fs *FileSystem) tryKernelLookup(name string, parent fuseops.InodeID) (*fuseops.ChildInodeEntry, bool) {
	if globalKernelLM == nil {
		return nil, false
	}
	switch {
	case parent == fuseops.RootInodeID && name == kernelDirName:
		return fs.kernelChildEntry(kernelDirInode, true), true
	case parent == kernelDirInode:
		switch name {
		case "agents": return fs.kernelChildEntry(kernelAgentsDirInode, true), true
		case "tasks":  return fs.kernelChildEntry(kernelTasksDirInode, true), true
		case "resources": return fs.kernelChildEntry(kernelResourcesDirInode, true), true
		case "channels": return fs.kernelChildEntry(kernelChannelsDirInode, true), true
		case "config": return fs.kernelChildEntry(kernelConfigInode, false), true
		case "restart": return fs.kernelChildEntry(kernelRestartInode, false), true
		}
	case parent == kernelAgentsDirInode:
		if a, ok := globalKernelLM.Get(kernel.AgentID(name)); ok {
			return fs.kernelChildEntry(kernelAgentInode(a.ID), true), true
		}
		if name == "spawn" {
			return fs.kernelChildEntry(kernelAgentSpawnInode, false), true
		}
		if name == "summary" {
			return fs.kernelChildEntry(kernelAgentSummaryInode, false), true
		}
	case parent == kernelTasksDirInode:
		switch name {
		case "pending", "running", "completed", "failed", "submit":
			return fs.kernelChildEntry(kernelTasksDirInode+0x10+hashInode(name)&0xF, false), true
		}
	case parent == kernelResourcesDirInode:
		switch name {
		case "usage": return fs.kernelChildEntry(kernelResourcesUsageInode, false), true
		case "limits": return fs.kernelChildEntry(kernelResourcesLimitsInode, false), true
		}
	case isKernelAgentDirInode(parent):
		a := kernelAgentFromDirInode(parent)
		if a == nil {
			return nil, false
		}
		switch name {
		case "status": return fs.kernelChildEntry(parent|0x01, false), true
		case "type": return fs.kernelChildEntry(parent|0x02, false), true
		case "config": return fs.kernelChildEntry(parent|0x03, false), true
		case "resources": return fs.kernelChildEntry(parent|0x04, false), true
		case "log": return fs.kernelChildEntry(parent|0x05, false), true
		case "suspend": return fs.kernelChildEntry(parent|0x06, false), true
		case "resume": return fs.kernelChildEntry(parent|0x07, false), true
		case "kill": return fs.kernelChildEntry(parent|0x08, false), true
		}
	}
	return nil, false
}

func (fs *FileSystem) kernelChildEntry(ino fuseops.InodeID, isDir bool) *fuseops.ChildInodeEntry {
	return &fuseops.ChildInodeEntry{
		Child: ino, Generation: 1,
		Attributes: fs.kernelAttr(ino, isDir), AttributesExpiration: time.Now().Add(5 * time.Second),
		EntryExpiration: time.Now().Add(5 * time.Second),
	}
}

func (fs *FileSystem) tryKernelGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isKernelInode(op.Inode) { return false }
	op.Attributes = fs.kernelAttr(op.Inode, fs.kernelIsDir(op.Inode))
	op.AttributesExpiration = time.Now().Add(5 * time.Second)
	return true
}

func (fs *FileSystem) tryKernelRead(op *fuseops.ReadFileOp) bool {
	if !fs.isKernelInode(op.Inode) { return false }
	var data []byte
	switch {
	case op.Inode == kernelAgentSummaryInode:
		agents := globalKernelLM.List("")
		var b strings.Builder
		for _, a := range agents {
			fmt.Fprintf(&b, "%s  %s  %s\n", a.ID, a.Type, a.State)
		}
		data = []byte(b.String())
	case op.Inode == kernelAgentSpawnInode:
		data = []byte("# Spawn Agent\n\necho '{\"type\":\"code-reviewer\",\"config\":{}}' > @kernel/agents/spawn\n")
	case op.Inode == kernelTasksSubmitInode:
		data = []byte("# Submit Task\n\necho '{\"name\":\"my task\",\"priority\":80}' > @kernel/tasks/submit\n")
	case op.Inode == kernelConfigInode:
		data = []byte("# Kernel config\nmax_agents: 50\n")
	case op.Inode == kernelResourcesUsageInode:
		summary := globalKernelRM.Usage()
		d, _ := json.MarshalIndent(summary, "", "  ")
		data = d
	case op.Inode == kernelResourcesLimitsInode:
		d, _ := json.MarshalIndent(kernel.DefaultResourceLimits(), "", "  ")
		data = d
	case op.Inode == kernelRestartInode:
		data = []byte("echo '1' > @kernel/restart to restart kernel\n")
	default:
		if agentSub, ok := fs.kernelAgentSubInode(op.Inode); ok {
			data = fs.kernelAgentSubContent(agentSub.agent, agentSub.sub)
		} else {
			return false
		}
	}
	if op.Offset >= int64(len(data)) { op.BytesRead = 0; return true }
	end := int(op.Offset) + len(op.Dst)
	if end > len(data) { end = len(data) }
	op.BytesRead = copy(op.Dst, data[op.Offset:end])
	return true
}

func (fs *FileSystem) tryKernelWrite(op *fuseops.WriteFileOp) bool {
	if !fs.isKernelInode(op.Inode) || op.Offset != 0 { return false }
	content := strings.TrimSpace(string(op.Data))
	switch {
	case op.Inode == kernelAgentSpawnInode:
		var req struct{ Type string; Config kernel.AgentConfig }
		json.Unmarshal([]byte(content), &req)
		id, _ := globalKernelLM.Spawn(kernel.AgentType(req.Type), req.Config)
		_ = id
		return true
	case op.Inode == kernelTasksSubmitInode:
		var t kernel.Task
		json.Unmarshal([]byte(content), &t)
		globalKernelS.Submit(t)
		return true
	case op.Inode == kernelRestartInode:
		return true
	default:
		if agentSub, ok := fs.kernelAgentSubInode(op.Inode); ok {
			switch agentSub.sub {
			case 0x06:
				globalKernelLM.Suspend(agentSub.agent)
			case 0x07:
				globalKernelLM.Resume(agentSub.agent)
			case 0x08:
				globalKernelLM.Kill(agentSub.agent)
			}
			return true
		}
	}
	return false
}

func (fs *FileSystem) tryKernelOpenDir(op *fuseops.OpenDirOp) bool {
	if !fs.isKernelInode(op.Inode) || !fs.kernelIsDir(op.Inode) { return false }
	entries := make([]fuseutil.Dirent, 0); o := 0
	add := func(ino fuseops.InodeID, name string, isDir bool) {
		o++; entries = append(entries, fuseutil.Dirent{Offset: fuseops.DirOffset(o), Inode: ino, Name: name, Type: kernelDirentType(isDir)})
	}
	switch op.Inode {
	case kernelDirInode:
		add(kernelAgentsDirInode, "agents", true)
		add(kernelTasksDirInode, "tasks", true)
		add(kernelResourcesDirInode, "resources", true)
		add(kernelChannelsDirInode, "channels", true)
		add(kernelConfigInode, "config", false)
		add(kernelRestartInode, "restart", false)
	case kernelAgentsDirInode:
		add(kernelAgentSpawnInode, "spawn", false)
		add(kernelAgentSummaryInode, "summary", false)
		for _, a := range globalKernelLM.List("") {
			add(kernelAgentInode(a.ID), string(a.ID), true)
		}
	case kernelTasksDirInode:
		add(kernelTasksSubmitInode, "submit", false)
		add(kernelTasksDirInode+0x10, "pending", false)
		add(kernelTasksDirInode+0x11, "running", false)
		add(kernelTasksDirInode+0x12, "completed", false)
		add(kernelTasksDirInode+0x13, "failed", false)
	case kernelResourcesDirInode:
		add(kernelResourcesUsageInode, "usage", false)
		add(kernelResourcesLimitsInode, "limits", false)
	case kernelChannelsDirInode:
		for _, ch := range globalKernelIPC.Channels() {
			add(kernelChannelsDirInode+hashInode(ch)&0xFF, ch, true)
		}
	default:
		if isKernelAgentDirInode(op.Inode) {
			for _, s := range []string{"status","type","config","resources","log","suspend","resume","kill"} {
				add(op.Inode+0x01+hashInode(s)&0x0F, s, false)
			}
		}
		return false
	}
	handle := fs.allocHandle()
	fs.handleMu.Lock(); fs.dirs[handle] = &dirHandle{entries: entries}; fs.handleMu.Unlock()
	op.Handle = handle
	return true
}

func (fs *FileSystem) kernelAttr(inode fuseops.InodeID, isDir bool) fuseops.InodeAttributes {
	mode := os.FileMode(0444); var size uint64
	if isDir { mode, size = 0555|os.ModeDir, 4096 } else { mode = 0644 }
	return fuseops.InodeAttributes{Size: size, Nlink: 1, Mode: mode,
		Atime: time.Now(), Mtime: time.Now(), Ctime: time.Now(),
		Uid: uint32(os.Getuid()), Gid: uint32(os.Getgid())}
}

func (fs *FileSystem) kernelIsDir(inode fuseops.InodeID) bool {
	switch inode {
	case kernelDirInode, kernelAgentsDirInode, kernelTasksDirInode, kernelResourcesDirInode, kernelChannelsDirInode:
		return true
	}
	return isKernelAgentDirInode(inode)
}

func isKernelAgentDirInode(inode fuseops.InodeID) bool {
	return inode >= kernelAgentInodeBase && inode < kernelAgentInodeBase+0x100000 && (inode&0xF) == 0
}

type kernelAgentSub struct {
	agent kernel.AgentID
	sub   fuseops.InodeID
}

func (fs *FileSystem) kernelAgentSubInode(inode fuseops.InodeID) (kernelAgentSub, bool) {
	if inode < kernelAgentInodeBase || inode >= kernelAgentInodeBase+0x100000 { return kernelAgentSub{}, false }
	sub := inode & 0xF
	if sub == 0 { return kernelAgentSub{}, false }
	for _, a := range globalKernelLM.List("") {
		if kernelAgentInode(a.ID) == inode&^fuseops.InodeID(0xFFFFF) {
			return kernelAgentSub{agent: a.ID, sub: sub}, true
		}
	}
	return kernelAgentSub{}, false
}

func kernelAgentFromDirInode(inode fuseops.InodeID) *kernel.Agent {
	for _, a := range globalKernelLM.List("") {
		if kernelAgentInode(a.ID) == inode {
			return a
		}
	}
	return nil
}

func (fs *FileSystem) kernelAgentSubContent(agentID kernel.AgentID, sub fuseops.InodeID) []byte {
	a, ok := globalKernelLM.Get(agentID)
	if !ok { return nil }
	switch sub {
	case 0x01: return []byte(string(a.State) + "\n")
	case 0x02: return []byte(string(a.Type) + "\n")
	case 0x03: d, _ := json.MarshalIndent(a.Config, "", "  "); return d
	case 0x04: d, _ := json.MarshalIndent(a.Resources, "", "  "); return d
	case 0x05:
		var b strings.Builder
		for _, l := range a.Log {
			fmt.Fprintf(&b, "[%s] %s: %s\n", l.Timestamp.Format("15:04:05"), l.Level, l.Message)
		}
		return []byte(b.String())
	case 0x06: return []byte("echo '1' > suspend to suspend agent\n")
	case 0x07: return []byte("echo '1' > resume to resume agent\n")
	case 0x08: return []byte("echo '1' > kill to terminate agent\n")
	}
	return nil
}

func kernelDirentType(isDir bool) fuseutil.DirentType {
	if isDir { return fuseutil.DT_Directory }
	return fuseutil.DT_File
}

func (fs *FileSystem) isKernelStateWritable(inode fuseops.InodeID) bool {
	return inode == kernelAgentSpawnInode || inode == kernelTasksSubmitInode || inode == kernelRestartInode ||
		(inode >= kernelAgentInodeBase && inode < kernelAgentInodeBase+0x100000 && (inode&0xF) >= 0x06)
}
