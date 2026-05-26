package provider

import "context"

// Provider is the unified interface for any LLM API backend.
type Provider interface {
	Name() string
	Models() []Model
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Ping(ctx context.Context) error
	SetAPIKey(key string)
}

// Capability describes a model feature.
type Capability string

const (
	CapText            Capability = "text"
	CapCode            Capability = "code"
	CapVision          Capability = "vision"
	CapReasoning       Capability = "reasoning"
	CapFunctionCalling Capability = "function_calling"
	CapStreaming       Capability = "streaming"
)

// Model describes a single model offered by a provider.
type Model struct {
	ID           string       `yaml:"id" json:"id"`
	Name         string       `yaml:"name" json:"name"`
	Provider     string       `yaml:"provider" json:"provider"`
	Capabilities []Capability `yaml:"capabilities" json:"capabilities"`
}

// Message is a single chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is a provider-agnostic chat request.
type ChatRequest struct {
	Model       string
	Messages    []Message
	Stream      bool
	Temperature float64
	MaxTokens   int
	TopP        float64
	Extra       map[string]any
}

// Usage reports token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse is a provider-agnostic chat response.
type ChatResponse struct {
	Content string `json:"content"`
	Model   string `json:"model"`
	Usage   Usage  `json:"usage"`
}

// ProviderConfig holds configuration for a single provider.
type ProviderConfig struct {
	Name     string            `yaml:"name"`
	Type     string            `yaml:"type"`
	APIKey   string            `yaml:"api_key,omitempty"`
	BaseURL  string            `yaml:"base_url,omitempty"`
	Models   []string          `yaml:"models,omitempty"`
	Priority int               `yaml:"priority"`
	Tags     map[string]string `yaml:"tags,omitempty"`
	Disabled bool              `yaml:"disabled,omitempty"`
}

// ProviderInfo is a summary returned for listing.
type ProviderInfo struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Models   []string `json:"models"`
	Priority int      `json:"priority"`
	Disabled bool     `json:"disabled"`
	Healthy  bool     `json:"healthy"`
}

// ProviderKeyInfo is a masked key entry for listing.
type ProviderKeyInfo struct {
	Provider string `json:"provider"`
	Masked   string `json:"masked"`
}

// HealthResult reports the health-check outcome for a provider.
type HealthResult struct {
	Provider string `json:"provider"`
	Healthy  bool   `json:"healthy"`
	Latency  string `json:"latency,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ConfigFile is the top-level structure of providers.yaml.
type ConfigFile struct {
	Providers []ProviderConfig `yaml:"providers"`
	Agents    AgentsConfig     `yaml:"agents"`
	Router    RouterConfig     `yaml:"router"`
}

// AgentsConfig holds agent-level provider settings.
type AgentsConfig struct {
	Default string `yaml:"default"`
}

// RouterConfig holds routing strategy settings.
type RouterConfig struct {
	Strategy      string   `yaml:"strategy"` // priority | latency | random
	Fallback      bool     `yaml:"fallback"`
	FallbackOrder []string `yaml:"fallback_order"`
}
