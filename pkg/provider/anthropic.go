package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const anthropicMessagesEndpoint = "https://api.anthropic.com/v1/messages"

// AnthropicProvider implements the Anthropic Messages API.
type AnthropicProvider struct {
	config  ProviderConfig
	client  *http.Client
	baseURL string
}

// NewAnthropicProvider creates an Anthropic provider from config.
func NewAnthropicProvider(config ProviderConfig) *AnthropicProvider {
	if config.BaseURL == "" {
		config.BaseURL = anthropicMessagesEndpoint
	}
	return &AnthropicProvider{
		config:  config,
		client:  &http.Client{Timeout: 60 * time.Second},
		baseURL: config.BaseURL,
	}
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) SetAPIKey(key string) {
	p.config.APIKey = key
}

func (p *AnthropicProvider) Models() []Model {
	return []Model{
		{ID: "claude-opus-4-7", Name: "Claude Opus 4.7", Provider: "anthropic",
			Capabilities: []Capability{CapText, CapCode, CapVision, CapReasoning, CapFunctionCalling, CapStreaming}},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Provider: "anthropic",
			Capabilities: []Capability{CapText, CapCode, CapVision, CapFunctionCalling, CapStreaming}},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Provider: "anthropic",
			Capabilities: []Capability{CapText, CapCode, CapStreaming}},
	}
}

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	systemMsg := ""
	var messages []map[string]any
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemMsg = m.Content
			continue
		}
		messages = append(messages, map[string]any{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	body := map[string]any{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   messages,
	}
	if systemMsg != "" {
		body["system"] = systemMsg
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if req.TopP > 0 {
		body["top_p"] = req.TopP
	}
	for k, v := range req.Extra {
		body[k] = v
	}

	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	content := ""
	if c, ok := resp["content"].([]any); ok && len(c) > 0 {
		if block, ok := c[0].(map[string]any); ok {
			if text, ok := block["text"].(string); ok {
				content = text
			}
		}
	}

	model, _ := resp["model"].(string)
	usage := Usage{}
	if u, ok := resp["usage"].(map[string]any); ok {
		if pt, ok := u["input_tokens"].(float64); ok {
			usage.PromptTokens = int(pt)
		}
		if ct, ok := u["output_tokens"].(float64); ok {
			usage.CompletionTokens = int(ct)
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return &ChatResponse{Content: content, Model: model, Usage: usage}, nil
}

func (p *AnthropicProvider) Ping(ctx context.Context) error {
	ep := p.baseURL
	if ep == anthropicMessagesEndpoint {
		// Use a test endpoint.
		ep = "https://api.anthropic.com/v1/models"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", ep, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ping: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (p *AnthropicProvider) doRequest(ctx context.Context, body map[string]any) (map[string]any, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respData))
	}

	var result map[string]any
	if err := json.Unmarshal(respData, &result); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return result, nil
}
