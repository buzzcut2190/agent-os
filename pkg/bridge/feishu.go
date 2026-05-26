package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// FeishuBridge implements the Feishu (Lark) bot messaging API.
type FeishuBridge struct {
	config     Config
	status     Status
	accessToken string
	tokenExpiry time.Time
	mu         sync.RWMutex
	client     *http.Client
}

// NewFeishuBridge creates a Feishu bridge.
func NewFeishuBridge(config Config) *FeishuBridge {
	return &FeishuBridge{
		config: config,
		status: Disconnected,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (b *FeishuBridge) Name() string { return b.config.Name }
func (b *FeishuBridge) Type() string { return "feishu" }

func (b *FeishuBridge) Connect(ctx context.Context) error {
	if b.config.AppID == "" || b.config.AppSecret == "" {
		return fmt.Errorf("feishu requires app_id and app_secret")
	}
	if err := b.refreshToken(ctx); err != nil {
		return fmt.Errorf("feishu connect: %w", err)
	}
	b.status = Connected
	return nil
}

func (b *FeishuBridge) Disconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status = Disconnected
	b.accessToken = ""
	return nil
}

func (b *FeishuBridge) Send(ctx context.Context, target string, content string) error {
	if b.status != Connected {
		return fmt.Errorf("feishu bridge not connected")
	}
	token, err := b.getToken(ctx)
	if err != nil {
		return err
	}
	body := map[string]any{
		"receive_id": target,
		"msg_type":   "text",
		"content":    fmt.Sprintf(`{"text":"%s"}`, content),
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id",
		bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("feishu send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respData, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("feishu: HTTP %d: %s", resp.StatusCode, string(respData))
	}
	return nil
}

func (b *FeishuBridge) Status() Status {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

func (b *FeishuBridge) getToken(ctx context.Context) (string, error) {
	b.mu.RLock()
	if b.accessToken != "" && time.Now().Before(b.tokenExpiry) {
		defer b.mu.RUnlock()
		return b.accessToken, nil
	}
	b.mu.RUnlock()
	return "", b.refreshToken(ctx)
}

func (b *FeishuBridge) refreshToken(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	body := map[string]string{
		"app_id":     b.config.AppID,
		"app_secret": b.config.AppSecret,
	}
	data, _ := json.Marshal(body)
	resp, err := b.client.Post(
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		"application/json", bytes.NewReader(data))
	if err != nil {
		b.status = Error
		return err
	}
	defer resp.Body.Close()
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			AccessToken string `json:"tenant_access_token"`
			Expire      int    `json:"expire"`
		} `json:"tenant_access_token_response"` // actual API uses different nesting
	}
	// Try alternate parsing for feishu which uses flat response.
	respData, _ := io.ReadAll(resp.Body)
	var altResult struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.Unmarshal(respData, &altResult); err != nil {
		b.status = Error
		return fmt.Errorf("parse token: %w", err)
	}
	_ = result
	if altResult.Code != 0 {
		b.status = Error
		return fmt.Errorf("feishu auth: code=%d msg=%s", altResult.Code, altResult.Msg)
	}
	b.accessToken = altResult.TenantAccessToken
	b.tokenExpiry = time.Now().Add(time.Duration(altResult.Expire) * time.Second)
	b.status = Connected
	return nil
}
