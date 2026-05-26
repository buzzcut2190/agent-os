// Package plugin provides a plugin SDK for extending agentfs behavior.
// Plugins can hook into filesystem operations (read, write, close) and
// are managed through a central registry.
package plugin

import "context"

// Plugin is the interface all plugins must implement.
// Plugins can intercept and transform filesystem I/O operations,
// enabling features like auto-formatting, linting, and custom hooks.
type Plugin interface {
	// Name returns the unique plugin identifier (e.g. "auto-format").
	Name() string

	// Version returns the semantic version string (e.g. "1.0.0").
	Version() string

	// Init is called after registration to configure the plugin.
	// cfg contains plugin-specific settings from the config file.
	Init(cfg map[string]any) error

	// OnRead is called before a file read completes. The plugin can
	// transform the returned data (e.g. inject git annotations).
	OnRead(ctx context.Context, path string, data []byte) ([]byte, error)

	// OnWrite is called before a file write is persisted. The plugin can
	// transform the data being written (e.g. run a formatter).
	OnWrite(ctx context.Context, path string, data []byte) ([]byte, error)

	// OnClose is called during filesystem teardown for cleanup.
	OnClose(ctx context.Context) error
}

// PluginInfo describes a registered plugin's metadata and state.
type PluginInfo struct {
	Name    string
	Version string
	Enabled bool
	Config  map[string]any
}
