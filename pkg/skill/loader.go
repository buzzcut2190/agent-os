package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"

	"gopkg.in/yaml.v3"
)

// LoadFromDir scans a directory for skill subdirectories (each containing a
// skill.yaml) and registers them. Source labels the origin for priority
// resolution ("builtin", "global", "user", "project").
func (e *Engine) LoadFromDir(dir string, source string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read skill dir %s: %w", dir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, entry.Name())
		skillFS := os.DirFS(skillDir)
		def, err := loadSkill(skillFS, source)
		if err != nil {
			// Graceful degradation: log and skip broken skills.
			e.mu.Lock()
			e.skills[entry.Name()] = &SkillDefinition{
				Manifest: SkillManifest{Name: entry.Name()},
				State:    SkillError,
				LoadError: err,
				Source:   source,
			}
			e.mu.Unlock()
			continue
		}
		e.mu.Lock()
		e.skills[def.Manifest.Name] = def
		e.mu.Unlock()
	}
	return nil
}

// LoadFromFS loads skills from an fs.FS where each top-level directory is a
// skill with a skill.yaml inside. Used for embedded builtin skills.
func (e *Engine) LoadFromFS(fsys fs.FS, source string) error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("read embed skill fs: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subFS, err := fs.Sub(fsys, entry.Name())
		if err != nil {
			continue
		}
		def, err := loadSkill(subFS, source)
		if err != nil {
			e.mu.Lock()
			e.skills[entry.Name()] = &SkillDefinition{
				Manifest:  SkillManifest{Name: entry.Name()},
				State:     SkillError,
				LoadError: err,
				Source:    source,
			}
			e.mu.Unlock()
			continue
		}
		e.mu.Lock()
		e.skills[def.Manifest.Name] = def
		e.mu.Unlock()
	}
	return nil
}

// loadSkill parses skill.yaml from an fs.FS and compiles context/prompt templates.
func loadSkill(fsys fs.FS, source string) (*SkillDefinition, error) {
	data, err := fs.ReadFile(fsys, "skill.yaml")
	if err != nil {
		return nil, fmt.Errorf("read skill.yaml: %w", err)
	}
	var m SkillManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse skill.yaml: %w", err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("skill.yaml missing required field 'name'")
	}
	if m.ContextFile == "" {
		m.ContextFile = resolveContextFile(fsys, m.Lang)
	}
	if m.PromptFile == "" {
		m.PromptFile = resolvePromptFile(fsys, m.Lang)
	}

	def := &SkillDefinition{
		Manifest: m,
		FS:       fsys,
		State:    SkillInactive,
		Source:   source,
	}

	// Compile context template.
	ctxData, err := fs.ReadFile(fsys, m.ContextFile)
	if err != nil {
		def.LoadError = fmt.Errorf("read %s: %w", m.ContextFile, err)
		return def, nil // still usable, just no context
	}
	ctxTmpl, err := template.New(m.Name + "-context").Parse(string(ctxData))
	if err != nil {
		def.LoadError = fmt.Errorf("parse %s: %w", m.ContextFile, err)
		return def, nil
	}
	def.ContextTmpl = ctxTmpl

	// Compile prompt template.
	promptData, err := fs.ReadFile(fsys, m.PromptFile)
	if err != nil {
		// Prompt is optional — only context is critical.
		return def, nil
	}
	promptTmpl, err := template.New(m.Name + "-prompt").Parse(string(promptData))
	if err != nil {
		return def, nil
	}
	def.PromptTmpl = promptTmpl

	return def, nil
}

// resolveContextFile picks the best context file based on language preference.
func resolveContextFile(fsys fs.FS, lang string) string {
	order := []string{"context.zh.md", "context.en.md", "context.md"}
	if lang == "en" {
		order = []string{"context.en.md", "context.zh.md", "context.md"}
	}
	for _, f := range order {
		if _, err := fs.Stat(fsys, f); err == nil {
			return f
		}
	}
	return "context.md"
}

// resolvePromptFile picks the best prompt file based on language preference.
func resolvePromptFile(fsys fs.FS, lang string) string {
	order := []string{"prompt.zh.md", "prompt.en.md", "prompt.md"}
	if lang == "en" {
		order = []string{"prompt.en.md", "prompt.zh.md", "prompt.md"}
	}
	for _, f := range order {
		if _, err := fs.Stat(fsys, f); err == nil {
			return f
		}
	}
	return "prompt.md"
}
