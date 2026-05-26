package plugin

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PluginYAML is the on-disk format for a script-based plugin definition.
type PluginYAML struct {
	Name        string         `yaml:"name"`
	Version     string         `yaml:"version"`
	Description string         `yaml:"description,omitempty"`
	Hooks       PluginHooks    `yaml:"hooks"`
	Config      map[string]any `yaml:"config,omitempty"`
}

// PluginHooks maps hook names to shell commands. Supported keys:
//
//	on_read   — command whose stdout replaces file read output
//	on_write  — command run before writing (receives path as arg)
//	on_close  — cleanup command
//	on_context — command that contributes to @context output
type PluginHooks struct {
	OnRead    string `yaml:"on_read,omitempty"`
	OnWrite   string `yaml:"on_write,omitempty"`
	OnClose   string `yaml:"on_close,omitempty"`
	OnContext string `yaml:"on_context,omitempty"`
}

// scriptPlugin wraps an external command so it satisfies the Plugin interface.
type scriptPlugin struct {
	name    string
	version string
	cfg     map[string]any
	hooks   PluginHooks
}

func (s *scriptPlugin) Name() string                             { return s.name }
func (s *scriptPlugin) Version() string                          { return s.version }
func (s *scriptPlugin) Init(cfg map[string]any) error            { s.cfg = cfg; return nil }
func (s *scriptPlugin) OnClose(ctx context.Context) error        { _, err := s.runHook(ctx, s.hooks.OnClose, "", nil); return err }
func (s *scriptPlugin) OnRead(ctx context.Context, path string, data []byte) ([]byte, error) {
	return s.execWithData(ctx, s.hooks.OnRead, path, data)
}
func (s *scriptPlugin) OnWrite(ctx context.Context, path string, data []byte) ([]byte, error) {
	return s.execWithData(ctx, s.hooks.OnWrite, path, data)
}

func (s *scriptPlugin) execWithData(ctx context.Context, cmdTemplate, path string, data []byte) ([]byte, error) {
	if cmdTemplate == "" {
		return data, nil
	}
	cmd := interpolate(cmdTemplate, path)
	return s.runHook(ctx, cmd, path, data)
}

func (s *scriptPlugin) runHook(ctx context.Context, cmdLine, path string, stdin []byte) ([]byte, error) {
	if cmdLine == "" {
		return stdin, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return stdin, nil
	}

	//nolint:gosec // command comes from trusted plugin config
	c := exec.CommandContext(ctx, parts[0], parts[1:]...)
	c.Stdin = strings.NewReader(string(stdin))
	out, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("plugin hook %q: %w (output: %s)", cmdLine, err, string(out))
	}
	return out, nil
}

// interpolate replaces {{.Path}} in the template with the actual path.
func interpolate(tmpl, path string) string {
	return strings.ReplaceAll(tmpl, "{{.Path}}", path)
}

// LoadFromDir scans a directory for plugin configs and registers them.
// It reads *.yaml files at the top level and also follows symlinks in
// an "enabled/" subdirectory to support active-plugin toggling.
func LoadFromDir(r *Registry, dir string) error {
	// Load directly from *.yaml files in dir.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read plugin dir %q: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".yaml" || filepath.Ext(e.Name()) == ".yml" {
			abs := filepath.Join(dir, e.Name())
			if err := LoadYAMLPlugin(r, abs); err != nil {
				log.Printf("plugin: skip %s: %v", abs, err)
			}
		}
	}

	// Load from enabled/ symlink directory.
	enabledDir := filepath.Join(dir, "enabled")
	if info, err := os.Stat(enabledDir); err == nil && info.IsDir() {
		links, err := os.ReadDir(enabledDir)
		if err != nil {
			return fmt.Errorf("read enabled dir: %w", err)
		}
		for _, l := range links {
			abs := filepath.Join(enabledDir, l.Name())
			if filepath.Ext(l.Name()) == ".yaml" || filepath.Ext(l.Name()) == ".yml" {
				if err := LoadYAMLPlugin(r, abs); err != nil {
					log.Printf("plugin: skip %s: %v", abs, err)
				}
			}
		}
	}

	return nil
}

// LoadYAMLPlugin reads a single plugin.yaml file and registers the
// scriptPlugin it describes into the Registry.
func LoadYAMLPlugin(r *Registry, yamlPath string) error {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", yamlPath, err)
	}

	var def PluginYAML
	if err := yaml.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("parse %s: %w", yamlPath, err)
	}

	if def.Name == "" {
		return fmt.Errorf("%s: missing required field 'name'", yamlPath)
	}

	p := &scriptPlugin{
		name:    def.Name,
		version: def.Version,
		hooks:   def.Hooks,
	}

	info := PluginInfo{
		Name:    def.Name,
		Version: def.Version,
		Config:  def.Config,
	}

	return r.Register(p, info)
}
