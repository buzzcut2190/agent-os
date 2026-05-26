package fs

import (
	"fmt"
	"sync/atomic"
)

// Limits tracks and enforces resource caps on open files and active
// sessions so that the FUSE daemon can apply back-pressure.
type Limits struct {
	MaxOpenFiles int32
	MaxSessions  int32

	openFiles      atomic.Int32
	activeSessions atomic.Int32
}

// NewLimits creates a Limits tracker with the given maximums.
func NewLimits(maxFiles, maxSessions int32) *Limits {
	return &Limits{
		MaxOpenFiles: maxFiles,
		MaxSessions:  maxSessions,
	}
}

// OpenFile atomically increments the open-file counter and returns an
// error if the limit has been reached.
func (l *Limits) OpenFile() error {
	n := l.openFiles.Add(1)
	if n > l.MaxOpenFiles {
		l.openFiles.Add(-1)
		return fmt.Errorf("too many open files: %d (max %d)", n, l.MaxOpenFiles)
	}
	return nil
}

// CloseFile atomically decrements the open-file counter.
func (l *Limits) CloseFile() {
	l.openFiles.Add(-1)
}

// StartSession atomically increments the session counter and returns an
// error if the limit has been reached.
func (l *Limits) StartSession() error {
	n := l.activeSessions.Add(1)
	if n > l.MaxSessions {
		l.activeSessions.Add(-1)
		return fmt.Errorf("too many active sessions: %d (max %d)", n, l.MaxSessions)
	}
	return nil
}

// EndSession atomically decrements the session counter.
func (l *Limits) EndSession() {
	l.activeSessions.Add(-1)
}

// Stats returns the current value of both resource counters.
func (l *Limits) Stats() (openFiles, activeSessions int32) {
	return l.openFiles.Load(), l.activeSessions.Load()
}
