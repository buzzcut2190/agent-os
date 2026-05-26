package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agent-os/agent-os/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Global provider state for MCP tools.
var (
	mcpRegistry *provider.Registry
	mcpRouter   *provider.Router
	mcpKeyStore *provider.KeyStore
)

// SetProviderState injects the provider components for MCP access.
func SetProviderState(reg *provider.Registry, r *provider.Router, ks *provider.KeyStore) {
	mcpRegistry = reg
	mcpRouter = r
	mcpKeyStore = ks
}

// RegisterProviderTools registers all provider-related MCP tools.
func RegisterProviderTools(srv *server.MCPServer) {
	registerListProviders(srv)
	registerListModels(srv)
	registerGetProvider(srv)
	registerSetDefaultModel(srv)
	registerSetAPIKey(srv)
	registerTestProvider(srv)
	registerSwitchProvider(srv)
}

func registerListProviders(srv *server.MCPServer) {
	tool := mcp.NewTool("list_providers",
		mcp.WithDescription("List all registered model providers with their models and health status."),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if mcpRegistry == nil {
			return mcp.NewToolResultText("[]"), nil
		}
		type entry struct {
			Name    string   `json:"name"`
			Models  []string `json:"models"`
			Healthy bool     `json:"healthy"`
		}
		var list []entry
		for _, p := range mcpRegistry.All() {
			models := make([]string, len(p.Models()))
			for i, m := range p.Models() {
				models[i] = m.ID
			}
			healthy := p.Ping(ctx) == nil
			list = append(list, entry{Name: p.Name(), Models: models, Healthy: healthy})
		}
		data, _ := json.MarshalIndent(list, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})
}

func registerListModels(srv *server.MCPServer) {
	tool := mcp.NewTool("list_models",
		mcp.WithDescription("List all available models across all providers."),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if mcpRouter == nil {
			return mcp.NewToolResultText("[]"), nil
		}
		type modelEntry struct {
			ID       string `json:"id"`
			Provider string `json:"provider"`
		}
		var models []modelEntry
		for _, m := range mcpRouter.GetAllModels() {
			models = append(models, modelEntry{ID: m.ID, Provider: m.Provider})
		}
		data, _ := json.MarshalIndent(models, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})
}

func registerGetProvider(srv *server.MCPServer) {
	tool := mcp.NewTool("get_provider",
		mcp.WithDescription("Get detailed info about a specific provider."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Provider name.")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}
		if mcpRegistry == nil {
			return mcp.NewToolResultError("no providers configured"), nil
		}
		p, ok := mcpRegistry.Get(name)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("provider %q not found", name)), nil
		}
		info := map[string]any{
			"name":    p.Name(),
			"models":  p.Models(),
			"healthy": p.Ping(ctx) == nil,
		}
		data, _ := json.MarshalIndent(info, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})
}

func registerSetDefaultModel(srv *server.MCPServer) {
	tool := mcp.NewTool("set_default_model",
		mcp.WithDescription("Set the default model for the agent."),
		mcp.WithString("model", mcp.Required(), mcp.Description("Model or provider name to set as default.")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		model := req.GetString("model", "")
		if model == "" {
			return mcp.NewToolResultError("model is required"), nil
		}
		if mcpRouter == nil {
			return mcp.NewToolResultError("no router configured"), nil
		}
		if err := mcpRouter.SetDefault(model); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("set default: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("default model set to %q", model)), nil
	})
}

func registerSetAPIKey(srv *server.MCPServer) {
	tool := mcp.NewTool("set_api_key",
		mcp.WithDescription("Set the API key for a provider. Returns masked confirmation."),
		mcp.WithString("provider", mcp.Required(), mcp.Description("Provider name.")),
		mcp.WithString("key", mcp.Required(), mcp.Description("API key.")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pName := req.GetString("provider", "")
		key := req.GetString("key", "")
		if pName == "" || key == "" {
			return mcp.NewToolResultError("provider and key are required"), nil
		}
		if mcpKeyStore == nil {
			return mcp.NewToolResultError("no keystore configured"), nil
		}
		if err := mcpKeyStore.Set(pName, key); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("set key: %v", err)), nil
		}
		masked := provider.Mask(key)
		return mcp.NewToolResultText(fmt.Sprintf("key set for %q: %s", pName, masked)), nil
	})
}

func registerTestProvider(srv *server.MCPServer) {
	tool := mcp.NewTool("test_provider",
		mcp.WithDescription("Test connection to a provider. If name is omitted, tests all."),
		mcp.WithString("name", mcp.Description("Provider name (omit to test all).")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if mcpRegistry == nil {
			return mcp.NewToolResultError("no providers configured"), nil
		}
		if name != "" {
			p, ok := mcpRegistry.Get(name)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("provider %q not found", name)), nil
			}
			err := p.Ping(ctx)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("%s: UNHEALTHY (%v)", name, err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("%s: HEALTHY", name)), nil
		}
		var results []provider.HealthResult
		for _, p := range mcpRegistry.All() {
			err := p.Ping(ctx)
			hr := provider.HealthResult{Provider: p.Name(), Healthy: err == nil}
			if err != nil {
				hr.Error = err.Error()
			}
			results = append(results, hr)
		}
		data, _ := json.MarshalIndent(results, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})
}

func registerSwitchProvider(srv *server.MCPServer) {
	tool := mcp.NewTool("switch_provider",
		mcp.WithDescription("Switch all agent requests to the specified provider."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Provider name to switch to.")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}
		if mcpRouter == nil || mcpRegistry == nil {
			return mcp.NewToolResultError("no router configured"), nil
		}
		if _, ok := mcpRegistry.Get(name); !ok {
			return mcp.NewToolResultError(fmt.Sprintf("provider %q not found", name)), nil
		}
		mcpRouter.SetDefault(name)
		return mcp.NewToolResultText(fmt.Sprintf("switched to provider %q", name)), nil
	})
}
