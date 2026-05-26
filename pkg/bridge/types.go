package bridge

import (
	"context"
	"time"
)

// Bridge is the interface for external communication channels.
type Bridge interface {
	Name() string
	Type() string
	Connect(ctx context.Context) error
	Disconnect() error
	Send(ctx context.Context, target string, content string) error
	Status() Status
}

// Status represents the connection state of a bridge.
type Status string

const (
	Disconnected Status = "disconnected"
	Connecting   Status = "connecting"
	Connected    Status = "connected"
	Error        Status = "error"
)

// Config holds bridge configuration.
type Config struct {
	Name     string `yaml:"name" json:"name"`
	Type     string `yaml:"type" json:"type"` // webhook, feishu, dingtalk, wechat, email
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	WebhookURL string `yaml:"webhook_url,omitempty" json:"webhook_url,omitempty"`
	AppID    string `yaml:"app_id,omitempty" json:"app_id,omitempty"`
	AppSecret string `yaml:"app_secret,omitempty" json:"app_secret,omitempty"`
}

// Message is a generic message sent/received through a bridge.
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	ThreadID  string    `json:"thread_id,omitempty"`
}

// Info is a summary returned for listing bridges.
type Info struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
}
