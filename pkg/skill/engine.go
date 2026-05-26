package skill

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"text/template"
)

// Engine manages skill lifecycle: loading, activation, deactivation, template rendering.
type Engine struct {
	mu      sync.RWMutex
	skills  map[string]*SkillDefinition
	active  map[string]bool
	sources []string
	stateDir string

	contextProvider func() (string, error)
	langProvider    func() string
	structProvider  func() string
	filesProvider   func() []string
	diffProvider    func() string
	projectName     string
}

// NewEngine creates a skill engine. skillDirs are load-source directories ordered
// by priority (later entries override earlier ones). stateDir is where activation
// state is persisted.
func NewEngine(stateDir string, projectName string, skillDirs ...string) *Engine {
	return &Engine{
		skills:    make(map[string]*SkillDefinition),
		active:    make(map[string]bool),
		sources:   skillDirs,
		stateDir:  stateDir,
		projectName: projectName,
	}
}

// SetContextProvider sets the function used to obtain @context summary.
func (e *Engine) SetContextProvider(fn func() (string, error)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.contextProvider = fn
}

// SetLangProvider sets the function used to detect the project's primary language.
func (e *Engine) SetLangProvider(fn func() string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.langProvider = fn
}

// SetStructProvider sets the function used to obtain project directory tree.
func (e *Engine) SetStructProvider(fn func() string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.structProvider = fn
}

// SetFilesProvider sets the function used to obtain project file list.
func (e *Engine) SetFilesProvider(fn func() []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.filesProvider = fn
}

// SetDiffProvider sets the function used to obtain the current diff.
func (e *Engine) SetDiffProvider(fn func() string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.diffProvider = fn
}

// LoadAll loads skills from all sources in priority order:
// project > user > global > builtin. Later loads override earlier ones.
func (e *Engine) LoadAll() error {
	// 1. Builtin skills (lowest priority).
	e.LoadFromFS(BuiltinSkillsFS(), "builtin")

	// 2. Global skills.
	if dir := globalSkillDir(); dir != "" {
		e.LoadFromDir(dir, "global")
	}

	// 3. User skills.
	if dir := userSkillDir(); dir != "" {
		e.LoadFromDir(dir, "user")
	}

	// 4. Project sources (highest priority, can override builtins).
	for _, src := range e.sources {
		if _, err := os.Stat(src); err == nil {
			e.LoadFromDir(src, "project")
		}
	}

	return nil
}

// List returns all registered skills as SkillInfo summaries.
func (e *Engine) List() []SkillInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.listLocked()
}

func (e *Engine) listLocked() []SkillInfo {
	names := make([]string, 0, len(e.skills))
	for n := range e.skills {
		names = append(names, n)
	}
	sort.Strings(names)

	result := make([]SkillInfo, 0, len(names))
	for _, n := range names {
		s := e.skills[n]
		result = append(result, SkillInfo{
			Name:        s.Manifest.Name,
			Description: s.Manifest.Description,
			Version:     s.Manifest.Version,
			Author:      s.Manifest.Author,
			Tags:        s.Manifest.Tags,
			State:       s.State,
			Source:      s.Source,
		})
	}
	return result
}

// Get returns the full SkillDefinition or an error if not found.
func (e *Engine) Get(name string) (*SkillDefinition, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.skills[name]
	if !ok {
		return nil, ErrSkillNotFound
	}
	return s, nil
}

// Activate activates a skill (idempotent).
func (e *Engine) Activate(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.skills[name]
	if !ok {
		return ErrSkillNotFound
	}
	s.State = SkillActive
	e.active[name] = true
	return nil
}

// Deactivate deactivates a skill (idempotent).
func (e *Engine) Deactivate(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.skills[name]
	if !ok {
		return ErrSkillNotFound
	}
	s.State = SkillInactive
	delete(e.active, name)
	return nil
}

// IsActive returns whether the named skill is currently active.
func (e *Engine) IsActive(name string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.active[name]
}

// Active returns SkillInfo for all currently active skills.
func (e *Engine) Active() []SkillInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names := make([]string, 0, len(e.active))
	for n := range e.active {
		names = append(names, n)
	}
	sort.Strings(names)

	result := make([]SkillInfo, 0, len(names))
	for _, n := range names {
		s := e.skills[n]
		if s == nil {
			continue
		}
		result = append(result, SkillInfo{
			Name:        s.Manifest.Name,
			Description: s.Manifest.Description,
			Version:     s.Manifest.Version,
			Author:      s.Manifest.Author,
			Tags:        s.Manifest.Tags,
			State:       s.State,
			Source:      s.Source,
		})
	}
	return result
}

// ActiveNames returns just the names of active skills.
func (e *Engine) ActiveNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names := make([]string, 0, len(e.active))
	for n := range e.active {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// GetContext renders and returns the skill's context.md template output.
func (e *Engine) GetContext(name string) (string, error) {
	s, err := e.Get(name)
	if err != nil {
		return "", err
	}
	if s.ContextTmpl == nil {
		if s.LoadError != nil {
			return "", s.LoadError
		}
		return "", fmt.Errorf("skill %s has no context template", name)
	}
	data := e.buildTemplateData(name)
	var buf bytes.Buffer
	if err := s.ContextTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render context for %s: %w", name, err)
	}
	return buf.String(), nil
}

// GetPrompt returns the skill's prompt.md. If rendered is true the template
// is executed; otherwise the raw template source is returned.
func (e *Engine) GetPrompt(name string, rendered bool) (string, error) {
	s, err := e.Get(name)
	if err != nil {
		return "", err
	}
	if s.PromptTmpl == nil {
		// Return raw file content as fallback.
		if s.FS != nil {
			data, readErr := fsReadFile(s.FS, s.Manifest.PromptFile)
			if readErr != nil {
				return "", fmt.Errorf("skill %s has no prompt: %w", name, readErr)
			}
			return string(data), nil
		}
		return "", fmt.Errorf("skill %s has no prompt", name)
	}
	if !rendered {
		// Return raw template.
		return templateSource(s.PromptTmpl), nil
	}
	data := e.buildTemplateData(name)
	var buf bytes.Buffer
	if err := s.PromptTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render prompt for %s: %w", name, err)
	}
	return buf.String(), nil
}

// GetMetadata returns the skill's metadata as JSON.
func (e *Engine) GetMetadata(name string) ([]byte, error) {
	s, err := e.Get(name)
	if err != nil {
		return nil, err
	}
	info := SkillInfo{
		Name:        s.Manifest.Name,
		Description: s.Manifest.Description,
		Version:     s.Manifest.Version,
		Author:      s.Manifest.Author,
		Tags:        s.Manifest.Tags,
		State:       s.State,
		Source:      s.Source,
	}
	return json.MarshalIndent(info, "", "  ")
}

// Discover scans a directory for new skills not yet registered.
func (e *Engine) Discover(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("discover dir %s: %w", dir, err)
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

	var discovered []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, exists := e.skills[entry.Name()]; !exists {
			// Check if directory contains a skill.yaml.
			yamlPath := filepath.Join(dir, entry.Name(), "skill.yaml")
			if _, err := os.Stat(yamlPath); err == nil {
				discovered = append(discovered, entry.Name())
			}
		}
	}
	sort.Strings(discovered)
	return discovered, nil
}

// Install copies a skill from srcPath into the first project source directory
// (or ~/.config/agentfs/skills/ if no project source), then reloads.
func (e *Engine) Install(srcPath, name string) error {
	destDir := userSkillDir()
	if len(e.sources) > 0 {
		destDir = e.sources[0]
	}
	skillDir := filepath.Join(destDir, name)
	if err := copyDir(srcPath, skillDir); err != nil {
		return fmt.Errorf("install skill %s: %w", name, err)
	}
	return e.LoadFromDir(destDir, "project")
}

// SaveState persists the current set of active skill names to a JSON file.
func (e *Engine) SaveState() error {
	if e.stateDir == "" {
		return nil
	}
	names := e.ActiveNames()
	if err := os.MkdirAll(e.stateDir, 0755); err != nil {
		return err
	}
	statePath := filepath.Join(e.stateDir, "skills-state.json")
	f, err := os.Create(statePath)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(names)
}

// LoadState restores activation state from the persisted JSON file.
func (e *Engine) LoadState() error {
	if e.stateDir == "" {
		return nil
	}
	statePath := filepath.Join(e.stateDir, "skills-state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range names {
		if s, ok := e.skills[n]; ok {
			s.State = SkillActive
			e.active[n] = true
		}
	}
	return nil
}

// buildTemplateData assembles the TemplateData for rendering.
func (e *Engine) buildTemplateData(skillName string) TemplateData {
	e.mu.RLock()
	defer e.mu.RUnlock()

	td := TemplateData{
		ProjectName: e.projectName,
		ActiveSkills: e.activeNamesLocked(),
		Custom:      make(map[string]any),
	}
	if s, ok := e.skills[skillName]; ok {
		td.Config = s.Manifest.Config
	}
	if e.contextProvider != nil {
		td.ContextSummary, _ = e.contextProvider()
	}
	if e.langProvider != nil {
		td.ProjectLang = e.langProvider()
	}
	if e.structProvider != nil {
		td.ProjectStructure = e.structProvider()
	}
	if e.filesProvider != nil {
		td.Files = e.filesProvider()
	}
	if e.diffProvider != nil {
		td.Diff = e.diffProvider()
	}
	return td
}

func (e *Engine) activeNamesLocked() []string {
	names := make([]string, 0, len(e.active))
	for n := range e.active {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// --- helpers ---

func globalSkillDir() string {
	// /usr/share/agentfs/skills on Linux.
	if _, err := os.Stat("/usr/share/agentfs/skills"); err == nil {
		return "/usr/share/agentfs/skills"
	}
	return ""
}

func userSkillDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "agentfs", "skills")
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}

func fsReadFile(fsys fs.FS, name string) ([]byte, error) {
	return fs.ReadFile(fsys, name)
}

// templateSource returns the underlying template string.
func templateSource(t *template.Template) string {
	if t == nil || t.Tree == nil {
		return ""
	}
	return t.Tree.Root.String()
}
