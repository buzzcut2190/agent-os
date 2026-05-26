package fs

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agent-os/agent-os/pkg/team"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
	"gopkg.in/yaml.v3"
)

func (fs *FileSystem) tryTasksOpenDir(op *fuseops.OpenDirOp) bool {
	if !fs.isTasksInode(op.Inode) {
		return false
	}
	if !fs.tasksIsDir(op.Inode) && op.Inode != tasksDirInode {
		return false
	}

	ts := fs.teamStore()
	var entries []fuseutil.Dirent
	offset := 0

	switch {
	case op.Inode == tasksDirInode:
		for _, ino := range tasksStaticDirentOrder {
			c := tasksStatusColumns[ino]
			offset++
			entries = append(entries, fuseutil.Dirent{
				Offset: fuseops.DirOffset(offset), Inode: ino,
				Name: c.name, Type: fuseutil.DT_Directory,
			})
		}
		offset++
		entries = append(entries, fuseutil.Dirent{
			Offset: fuseops.DirOffset(offset), Inode: tasksNewInode,
			Name: "new", Type: fuseutil.DT_File,
		})

	case op.Inode == tasksTodoInode:
		for _, t := range ts.ListTasksByStatus(team.TaskCreated) {
			offset++
			entries = append(entries, taskDirent(t, offset))
		}
		for _, t := range ts.ListTasksByStatus(team.TaskAssigned) {
			offset++
			entries = append(entries, taskDirent(t, offset))
		}

	case op.Inode == tasksInProgressInode:
		for _, t := range ts.ListTasksByStatus(team.TaskInProgress) {
			offset++
			entries = append(entries, taskDirent(t, offset))
		}

	case op.Inode == tasksReviewInode:
		for _, t := range ts.ListTasksByStatus(team.TaskReview) {
			offset++
			entries = append(entries, taskDirent(t, offset))
		}

	case op.Inode == tasksDoneInode:
		for _, t := range ts.ListTasksByStatus(team.TaskDone) {
			offset++
			entries = append(entries, taskDirent(t, offset))
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

func (fs *FileSystem) tasksAttr(inode fuseops.InodeID, isDir bool) fuseops.InodeAttributes {
	mode := os.FileMode(0444)
	var size uint64
	if isDir {
		mode = 0555 | os.ModeDir
		size = 4096
	} else if inode == tasksNewInode {
		mode = 0222
	} else if s := fs.taskFileSize(inode); s > 0 {
		size = s
	}
	return fuseops.InodeAttributes{
		Size: size, Nlink: 1, Mode: mode,
		Atime: time.Now(), Mtime: time.Now(), Ctime: time.Now(),
		Uid: uint32(os.Getuid()), Gid: uint32(os.Getgid()),
	}
}

func (fs *FileSystem) tasksIsDir(inode fuseops.InodeID) bool {
	switch inode {
	case tasksDirInode, tasksTodoInode, tasksInProgressInode, tasksReviewInode, tasksDoneInode:
		return true
	}
	return false
}

func (fs *FileSystem) taskFileContent(inode fuseops.InodeID) ([]byte, error) {
	t := fs.taskByInode(inode)
	if t == nil {
		return nil, fmt.Errorf("task not found for inode %d", inode)
	}
	return yaml.Marshal(t)
}

func (fs *FileSystem) taskFileSize(inode fuseops.InodeID) uint64 {
	data, err := fs.taskFileContent(inode)
	if err != nil {
		return 0
	}
	return uint64(len(data))
}

func (fs *FileSystem) taskByInode(inode fuseops.InodeID) *team.Task {
	ts := fs.teamStore()
	for _, st := range []team.TaskStatus{
		team.TaskCreated, team.TaskAssigned,
		team.TaskInProgress, team.TaskReview, team.TaskDone,
	} {
		for _, t := range ts.ListTasksByStatus(st) {
			if (dynTaskBase | (hashInode(t.ID) & 0xFFFFF)) == inode {
				return t
			}
		}
	}
	return nil
}

func taskFileName(t *team.Task) string {
	id := t.ID
	if len(id) > 8 {
		id = id[:8]
	}
	safe := strings.Map(func(r rune) rune {
		if r == ' ' || r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, t.Title)
	return fmt.Sprintf("%s_%s.yaml", id, safe)
}

func taskDirent(t *team.Task, offset int) fuseutil.Dirent {
	return fuseutil.Dirent{
		Offset: fuseops.DirOffset(offset),
		Inode:  dynTaskBase | (hashInode(t.ID) & 0xFFFFF),
		Name:   taskFileName(t),
		Type:   fuseutil.DT_File,
	}
}
