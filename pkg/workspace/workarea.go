package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ScratchArea is a temporary workspace area where agents can read and
// write scratch files. Files older than a configurable max age may be
// cleaned up.
type ScratchArea struct {
	root string
}

// NewScratchArea creates a ScratchArea rooted at the given path.
func NewScratchArea(root string) *ScratchArea {
	return &ScratchArea{root: root}
}

// Write saves data as a named file under the scratch directory.
// Parent directories are created automatically.
func (s *ScratchArea) Write(name string, data []byte) error {
	path := filepath.Join(s.root, name)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("scratch: write %s: %w", name, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("scratch: write %s: %w", name, err)
	}
	return nil
}

// Read returns the contents of a scratch file.
func (s *ScratchArea) Read(name string) ([]byte, error) {
	path := filepath.Join(s.root, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scratch: read %s: %w", name, err)
	}
	return data, nil
}

// List returns the names of all files currently in the scratch area,
// relative to the scratch root.
func (s *ScratchArea) List() ([]string, error) {
	var names []string
	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(s.root, path)
		if relErr != nil {
			return relErr
		}
		names = append(names, rel)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scratch: list: %w", err)
	}
	sort.Strings(names)
	return names, nil
}

// Cleanup removes scratch files whose modification time is older than
// maxAge. A zero maxAge removes all files.
func (s *ScratchArea) Cleanup(maxAge time.Duration) error {
	now := time.Now()
	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if maxAge > 0 {
			info, statErr := d.Info()
			if statErr != nil {
				return statErr
			}
			if now.Sub(info.ModTime()) < maxAge {
				return nil
			}
		}
		return os.Remove(path)
	})
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("scratch: cleanup: %w", err)
	}
	return nil
}

// ArtifactStore persists named work‑product artifacts under the
// artifacts directory.
type ArtifactStore struct {
	root string
}

// NewArtifactStore creates an ArtifactStore rooted at the given path.
func NewArtifactStore(root string) *ArtifactStore {
	return &ArtifactStore{root: root}
}

// Save persists an artifact as JSON. Size and CreatedAt are populated
// automatically if zero.
func (a *ArtifactStore) Save(name string, artifact Artifact) error {
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = time.Now()
	}
	if artifact.Size == 0 {
		artifact.Size = int64(len(artifact.Content))
	}
	// Enforce name on the struct to match the filename key.
	artifact.Name = name

	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("artifact store: marshal %s: %w", name, err)
	}

	path := filepath.Join(a.root, name+".json")
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("artifact store: save %s: %w", name, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("artifact store: save %s: %w", name, err)
	}
	return nil
}

// Get retrieves an artifact by name.
func (a *ArtifactStore) Get(name string) (*Artifact, error) {
	path := filepath.Join(a.root, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("artifact store: get %s: %w", name, err)
	}
	var art Artifact
	if err := json.Unmarshal(data, &art); err != nil {
		return nil, fmt.Errorf("artifact store: unmarshal %s: %w", name, err)
	}
	return &art, nil
}

// List returns copies of all stored artifacts sorted by name.
func (a *ArtifactStore) List() ([]*Artifact, error) {
	var arts []*Artifact
	err := filepath.WalkDir(a.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		var art Artifact
		if jsonErr := json.Unmarshal(data, &art); jsonErr != nil {
			return jsonErr
		}
		arts = append(arts, &art)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("artifact store: list: %w", err)
	}

	sort.Slice(arts, func(i, j int) bool {
		return arts[i].Name < arts[j].Name
	})
	return arts, nil
}
