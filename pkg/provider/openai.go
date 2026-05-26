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

// OpenAICompatibleProvider implements the OpenAI Chat Completions API.
// Also serves DeepSeek, GLM, Ollama via BaseURL configuration.
type OpenAICompatibleProvider struct {
	config  ProviderConfig
	client  *http.Client
	baseURL string
}

// NewOpenAICompatibleProvider creates a provider from config.
func NewOpenAICompatibleProvider(config ProviderConfig) *OpenAICompatibleProvider {
	return &OpenAICompatibleProvider{
		config:  config,
		client:  &http.Client{Timeout: 60 * time.Second},
		baseURL: config.BaseURL + "/chat/completions",
	}
}

func (p *OpenAICompatibleProvider) Name() string { return p.config.Name }

func (p *OpenAICompatibleProvider) SetAPIKey(key string) {
	p.config.APIKey = key
}

func (p *OpenAICompatibleProvider) Models() []Model {
	models := make([]Model, 0, len(p.config.Models))
	for _, id := range p.config.Models {
		models = append(models, Model{
			ID:       id,
			Name:     id,
			Provider: p.config.Name,
			Capabilities: []Capability{
				CapText, CapCode, CapFunctionCalling, CapStreaming,
			},
		})
	}
	return models
}

func (p *OpenAICompatibleProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	messages := make([]map[string]string, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = map[string]string{
			"role":    m.Role,
			"content": m.Content,
		}
	}

	body := map[string]any{
		"model":    req.Model,
		"messages": messages,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if req.TopP > 0 {
		body["top_p"] = req.TopP
	}
	if req.Stream {
		body["stream"] = true
	}
	for k, v := range req.Extra {
		body[k] = v
	}

	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	content := ""
	if choices, ok := resp["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				if c, ok := msg["content"].(string); ok {
					content = c
				}
			}
		}
	}

	model, _ := resp["model"].(string)
	usage := Usage{}
	if u, ok := resp["usage"].(map[string]any); ok {
		if pt, ok := u["prompt_tokens"].(float64); ok {
			usage.PromptTokens = int(pt)
		}
		if ct, ok := u["completion_tokens"].(float64); ok {
			usage.CompletionTokens = int(ct)
		}
		if tt, ok := u["total_tokens"].(float64); ok {
			usage.TotalTokens = int(tt)
		}
	}

	return &ChatResponse{Content: content, Model: model, Usage: usage}, nil
}

func (p *OpenAICompatibleProvider) Ping(ctx context.Context) error {
	// Use models endpoint for ping.
	ep := p.config.BaseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, "GET", ep, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
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

func (p *OpenAICompatibleProvider) doRequest(ctx context.Context, body map[string]any) (map[string]any, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)

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
