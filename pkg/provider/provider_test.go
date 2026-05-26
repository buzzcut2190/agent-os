package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mockProvider implements Provider for testing.
type mockProvider struct {
	name   string
	models []Model
	pingOK bool
	apiKey string
}

func (m *mockProvider) Name() string                                { return m.name }
func (m *mockProvider) Models() []Model                             { return m.models }
func (m *mockProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{Content: "mock response", Model: req.Model}, nil
}
func (m *mockProvider) SetAPIKey(key string) { m.apiKey = key }

func (m *mockProvider) Ping(ctx context.Context) error {
	if m.pingOK {
		return nil
	}
	return &testError{"ping failed"}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()
	p := &mockProvider{name: "test", pingOK: true}
	if err := r.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Get("test")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.Name() != "test" {
		t.Errorf("Name() = %q, want test", got.Name())
	}
}

func TestRegistryUnregister(t *testing.T) {
	r := NewRegistry()
	p := &mockProvider{name: "test"}
	r.Register(p)
	r.Unregister("test")
	if _, ok := r.Get("test"); ok {
		t.Error("expected test to be unregistered")
	}
}

func TestRegistryAll(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockProvider{name: "a"})
	r.Register(&mockProvider{name: "b"})
	all := r.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(all))
	}
}

func TestRegistryLoadSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "providers.yaml")

	cfg := &ConfigFile{
		Providers: []ProviderConfig{
			{Name: "test-openai", Type: "openai-compatible", BaseURL: "https://api.test.com/v1", Models: []string{"gpt-test"}},
		},
		Agents: AgentsConfig{Default: "test-openai"},
		Router: RouterConfig{Strategy: "priority", Fallback: true},
	}

	r := NewRegistry()
	if err := r.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file not created")
	}

	loaded, err := r.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.Agents.Default != "test-openai" {
		t.Errorf("default = %q, want test-openai", loaded.Agents.Default)
	}
}

func TestKeyStoreSetGet(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(filepath.Join(tmpDir, "keys.json"), nil)

	if err := ks.Set("test-provider", "sk-test-key-12345"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok := ks.Get("test-provider")
	if !ok {
		t.Fatal("key not found")
	}
	if got != "sk-test-key-12345" {
		t.Errorf("key = %q, want sk-test-key-12345", got)
	}
}

func TestKeyStoreMask(t *testing.T) {
	tests := []struct {
		key     string
		want    string
	}{
		{"sk-1234567890abcdef1234567890abcdef", "sk-12345...cdef"},
		{"short", "***"},
		{"abcdefghijkl", "***"},
	}
	for _, tt := range tests {
		got := Mask(tt.key)
		if got != tt.want {
			t.Errorf("Mask(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestKeyStoreDelete(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(filepath.Join(tmpDir, "keys.json"), nil)
	ks.Set("p", "key")
	ks.Delete("p")
	if _, ok := ks.Get("p"); ok {
		t.Error("key should be deleted")
	}
}

func TestKeyStoreList(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(filepath.Join(tmpDir, "keys.json"), nil)
	ks.Set("a", "sk-abcdefgh1234567890")
	ks.Set("b", "sk-short")

	list := ks.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(list))
	}
	for _, info := range list {
		if info.Masked == "" {
			t.Errorf("masked key for %s is empty", info.Provider)
		}
	}
}

func TestKeyStoreEncryption(t *testing.T) {
	tmpDir := t.TempDir()
	encKey := DeriveKey("test-passphrase-32bytes!!")
	ks := NewKeyStore(filepath.Join(tmpDir, "keys-enc.json"), encKey)
	ks.Set("enc-provider", "sk-secret-key")

	// Reload — should decrypt correctly.
	ks2 := NewKeyStore(filepath.Join(tmpDir, "keys-enc.json"), encKey)
	got, ok := ks2.Get("enc-provider")
	if !ok {
		t.Fatal("key not found after reload with encryption")
	}
	if got != "sk-secret-key" {
		t.Errorf("key = %q, want sk-secret-key", got)
	}
}

func TestKeyStoreFilePermission(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "keys.json")
	ks := NewKeyStore(keyPath, nil)
	ks.Set("p", "key")

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestRouterRoute(t *testing.T) {
	r := NewRouter([]Provider{
		&mockProvider{
			name:   "deepseek",
			models: []Model{{ID: "deepseek-chat", Provider: "deepseek"}},
		},
		&mockProvider{
			name:   "openai",
			models: []Model{{ID: "gpt-4", Provider: "openai"}},
		},
	})

	p, err := r.Route("deepseek-chat")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if p.Name() != "deepseek" {
		t.Errorf("routed to %s, want deepseek", p.Name())
	}

	// Route by provider name.
	p, err = r.Route("openai")
	if err != nil {
		t.Fatalf("Route by name: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("routed to %s, want openai", p.Name())
	}

	// Nonexistent.
	_, err = r.Route("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestRouterSetDefault(t *testing.T) {
	r := NewRouter([]Provider{
		&mockProvider{name: "a"},
		&mockProvider{name: "b"},
	})

	r.SetDefault("a")
	if d := r.GetDefault(); d != "a" {
		t.Errorf("default = %s, want a", d)
	}

	r.SetDefault("b")
	if d := r.GetDefault(); d != "b" {
		t.Errorf("default = %s, want b", d)
	}
}

func TestRouterList(t *testing.T) {
	r := NewRouter([]Provider{
		&mockProvider{name: "a", models: []Model{{ID: "m1"}, {ID: "m2"}}},
		&mockProvider{name: "b", models: []Model{{ID: "m3"}}},
	})

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(list))
	}
}

func TestRouterGetAllModels(t *testing.T) {
	r := NewRouter([]Provider{
		&mockProvider{name: "a", models: []Model{{ID: "m1"}, {ID: "m2"}}},
		&mockProvider{name: "b", models: []Model{{ID: "m3"}}},
	})

	models := r.GetAllModels()
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}
}
