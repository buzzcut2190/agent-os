package fs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// JournalEntry is a logged FUSE operation for crash recovery.
type JournalEntry struct {
	Op   string `json:"op"`
	Path string `json:"path"`
	Data []byte `json:"data,omitempty"`
}

// Journal provides write-ahead logging so that pending FUSE mutations can
// be replayed after an unclean shutdown.
type Journal struct {
	path string
	f    *os.File
}

// NewJournal opens (or creates) a journal file at the given path.
func NewJournal(path string) (*Journal, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("journal open: %w", err)
	}
	return &Journal{path: path, f: f}, nil
}

// Append writes a journal entry as a JSON line.
func (j *Journal) Append(e JournalEntry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("journal marshal: %w", err)
	}
	if _, err := j.f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("journal write: %w", err)
	}
	return nil
}

// Replay reads every entry from the journal and invokes cb for each.
// Entries that fail to unmarshal are skipped.
func (j *Journal) Replay(cb func(JournalEntry) error) error {
	if _, err := j.f.Seek(0, 0); err != nil {
		return fmt.Errorf("journal seek: %w", err)
	}
	scanner := bufio.NewScanner(j.f)
	for scanner.Scan() {
		var e JournalEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if err := cb(e); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// Truncate empties the journal and resets the file offset.
func (j *Journal) Truncate() error {
	if err := os.Truncate(j.path, 0); err != nil {
		return fmt.Errorf("journal truncate: %w", err)
	}
	if _, err := j.f.Seek(0, 0); err != nil {
		return fmt.Errorf("journal seek after truncate: %w", err)
	}
	return nil
}

// Close flushes and closes the underlying file.
func (j *Journal) Close() error {
	return j.f.Close()
}
