package skill

import (
	"embed"
	"io/fs"
)

//go:embed skills
var builtinSkillsFS embed.FS

// BuiltinSkillsFS returns the embedded filesystem containing all builtin skills.
func BuiltinSkillsFS() fs.FS {
	sub, _ := fs.Sub(builtinSkillsFS, "skills")
	return sub
}
