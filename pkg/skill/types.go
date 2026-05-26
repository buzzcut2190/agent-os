package skill

import (
	"io/fs"
	"text/template"
)

// SkillState represents the activation state of a skill.
type SkillState int

const (
	SkillInactive SkillState = iota
	SkillActive
	SkillError
)

func (s SkillState) String() string {
	switch s {
	case SkillActive:
		return "on"
	case SkillError:
		return "error"
	default:
		return "off"
	}
}

// ParseState converts a string to SkillState. Accepted values:
// "on", "active", "1", "true" → SkillActive
// "off", "inactive", "0", "false" → SkillInactive
// anything else → SkillError
func ParseState(s string) SkillState {
	switch s {
	case "on", "active", "1", "true":
		return SkillActive
	case "off", "inactive", "0", "false":
		return SkillInactive
	default:
		return SkillError
	}
}

// SkillManifest is the deserialized form of a skill.yaml file.
type SkillManifest struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Version     string            `yaml:"version"`
	Author      string            `yaml:"author"`
	Tags        []string          `yaml:"tags"`
	ContextFile string            `yaml:"context_file"`
	PromptFile  string            `yaml:"prompt_file"`
	Config      map[string]string `yaml:"config,omitempty"`
	Lang        string            `yaml:"lang,omitempty"`
}

// SkillDefinition is the in-memory representation of a skill.
type SkillDefinition struct {
	Manifest    SkillManifest
	FS          fs.FS
	State       SkillState
	ContextTmpl *template.Template
	PromptTmpl  *template.Template
	LoadError   error
	Source      string // "builtin", "project", "user", "global"
}

// TemplateData holds variables passed to template rendering.
type TemplateData struct {
	ProjectName      string
	ProjectLang      string
	ContextSummary   string
	ActiveSkills     []string
	ProjectStructure string
	Files            []string
	Diff             string
	Config           map[string]string
	Custom           map[string]any
}

// SkillInfo is a summary of a skill for listing.
type SkillInfo struct {
	Name        string     `json:"name" yaml:"name"`
	Description string     `json:"description" yaml:"description"`
	Version     string     `json:"version" yaml:"version"`
	Author      string     `json:"author" yaml:"author"`
	Tags        []string   `json:"tags" yaml:"tags"`
	State       SkillState `json:"state" yaml:"state"`
	Source      string     `json:"source" yaml:"source"`
}

// ErrSkillNotFound is returned when a skill name is not registered.
var ErrSkillNotFound = &SkillError_{msg: "skill not found"}

// SkillError_ is a simple error type for skill-related errors.
type SkillError_ struct {
	msg string
}

func (e *SkillError_) Error() string { return e.msg }

func skillError(msg string) error { return &SkillError_{msg: msg} }
