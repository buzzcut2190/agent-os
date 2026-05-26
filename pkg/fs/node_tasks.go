package fs

import (
	"fmt"
	"time"

	"github.com/agent-os/agent-os/pkg/team"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
	"gopkg.in/yaml.v3"
)

const (
	tasksDirName = "@tasks"

	tasksInodeBase        fuseops.InodeID = 0xFFFFF400
	tasksDirInode                         = tasksInodeBase + 0x000
	tasksTodoInode                        = tasksInodeBase + 0x001
	tasksInProgressInode                  = tasksInodeBase + 0x002
	tasksReviewInode                      = tasksInodeBase + 0x003
	tasksDoneInode                        = tasksInodeBase + 0x004
	tasksNewInode                         = tasksInodeBase + 0x005

	dynTaskBase fuseops.InodeID = 0xFFD00000
)

var tasksStatusColumns = map[fuseops.InodeID]struct {
	name   string
	status []team.TaskStatus
}{
	tasksTodoInode:        {"todo", []team.TaskStatus{team.TaskCreated, team.TaskAssigned}},
	tasksInProgressInode:  {"in_progress", []team.TaskStatus{team.TaskInProgress}},
	tasksReviewInode:      {"review", []team.TaskStatus{team.TaskReview}},
	tasksDoneInode:        {"done", []team.TaskStatus{team.TaskDone}},
}

var tasksStaticDirentOrder = []fuseops.InodeID{tasksTodoInode, tasksInProgressInode, tasksReviewInode, tasksDoneInode}

func (fs *FileSystem) isTasksInode(inode fuseops.InodeID) bool {
	return (inode >= tasksInodeBase && inode <= tasksNewInode) ||
		(inode >= dynTaskBase && inode < dynTaskBase+0x100000)
}

func (fs *FileSystem) tasksDirDirent(offset fuseops.DirOffset) fuseutil.Dirent {
	return fuseutil.Dirent{
		Offset: offset, Inode: tasksDirInode,
		Name: tasksDirName, Type: fuseutil.DT_Directory,
	}
}

func (fs *FileSystem) tryTasksLookup(name string, parent fuseops.InodeID) (*fuseops.ChildInodeEntry, bool) {
	ts := fs.teamStore()
	var ino fuseops.InodeID
	var isDir bool

	switch {
	case parent == fuseops.RootInodeID && name == tasksDirName:
		ino, isDir = tasksDirInode, true

	case parent == tasksDirInode:
		switch name {
		case "todo":
			ino, isDir = tasksTodoInode, true
		case "in_progress":
			ino, isDir = tasksInProgressInode, true
		case "review":
			ino, isDir = tasksReviewInode, true
		case "done":
			ino, isDir = tasksDoneInode, true
		case "new":
			ino = tasksNewInode
		default:
			return nil, false
		}

	case parent == tasksTodoInode:
		ino = fs.findTaskInode(ts, []team.TaskStatus{team.TaskCreated, team.TaskAssigned}, name)
		if ino == 0 {
			return nil, false
		}

	case parent == tasksInProgressInode:
		ino = fs.findTaskInode(ts, []team.TaskStatus{team.TaskInProgress}, name)
		if ino == 0 {
			return nil, false
		}

	case parent == tasksReviewInode:
		ino = fs.findTaskInode(ts, []team.TaskStatus{team.TaskReview}, name)
		if ino == 0 {
			return nil, false
		}

	case parent == tasksDoneInode:
		ino = fs.findTaskInode(ts, []team.TaskStatus{team.TaskDone}, name)
		if ino == 0 {
			return nil, false
		}

	default:
		return nil, false
	}

	return &fuseops.ChildInodeEntry{
		Child:                 ino,
		Generation:            1,
		Attributes:            fs.tasksAttr(ino, isDir),
		AttributesExpiration:  time.Now().Add(time.Second),
		EntryExpiration:       time.Now().Add(time.Second),
	}, true
}

func (fs *FileSystem) findTaskInode(ts *team.TeamStore, statuses []team.TaskStatus, name string) fuseops.InodeID {
	for _, st := range statuses {
		for _, t := range ts.ListTasksByStatus(st) {
			if name == taskFileName(t) {
				return dynTaskBase | (hashInode(t.ID) & 0xFFFFF)
			}
		}
	}
	return 0
}

func (fs *FileSystem) tryTasksGetAttr(op *fuseops.GetInodeAttributesOp) bool {
	if !fs.isTasksInode(op.Inode) {
		return false
	}
	isDir := fs.tasksIsDir(op.Inode)
	op.Attributes = fs.tasksAttr(op.Inode, isDir)
	op.AttributesExpiration = time.Now().Add(time.Second)
	return true
}

func (fs *FileSystem) tryTasksRead(op *fuseops.ReadFileOp) bool {
	if !fs.isTasksInode(op.Inode) {
		return false
	}
	var data []byte
	var err error

	switch {
	case op.Inode == tasksNewInode:
		data = []byte{}
	case op.Inode >= dynTaskBase && op.Inode < dynTaskBase+0x100000:
		data, err = fs.taskFileContent(op.Inode)
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

func (fs *FileSystem) tryTasksWrite(op *fuseops.WriteFileOp) bool {
	if !fs.isTasksInode(op.Inode) {
		return false
	}
	ts := fs.teamStore()

	switch {
	case op.Inode == tasksNewInode:
		if op.Offset != 0 {
			return false
		}
		_, err := ts.CreateTaskFromYAML(op.Data)
		return err == nil

	case op.Inode >= dynTaskBase && op.Inode < dynTaskBase+0x100000:
		task := fs.taskByInode(op.Inode)
		if task == nil {
			return false
		}
		var updated team.Task
		if err := yaml.Unmarshal(op.Data, &updated); err != nil {
			return false
		}
		if updated.Status != "" {
			_ = ts.UpdateTaskStatus(task.ID, updated.Status)
		}
		return true

	default:
		return false
	}
}
