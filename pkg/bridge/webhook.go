package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookBridge sends messages via HTTP POST to a webhook URL.
type WebhookBridge struct {
	config   Config
	status   Status
	client   *http.Client
}

// NewWebhookBridge creates a webhook bridge from config.
func NewWebhookBridge(config Config) *WebhookBridge {
	return &WebhookBridge{
		config: config,
		status: Disconnected,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (b *WebhookBridge) Name() string { return b.config.Name }
func (b *WebhookBridge) Type() string { return "webhook" }

func (b *WebhookBridge) Connect(ctx context.Context) error {
	if b.config.WebhookURL == "" {
		return fmt.Errorf("webhook_url is required")
	}
	b.status = Connected
	return nil
}

func (b *WebhookBridge) Disconnect() error {
	b.status = Disconnected
	return nil
}

func (b *WebhookBridge) Send(ctx context.Context, target string, content string) error {
	if b.status != Connected {
		return fmt.Errorf("bridge %s is not connected", b.config.Name)
	}
	msg := map[string]any{
		"target":  target,
		"content": content,
		"time":    time.Now().Format(time.RFC3339),
	}
	data, _ := json.Marshal(msg)
	req, err := http.NewRequestWithContext(ctx, "POST", b.config.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		b.status = Error
		return fmt.Errorf("webhook send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (b *WebhookBridge) Status() Status { return b.status }
