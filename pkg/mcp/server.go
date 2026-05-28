package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server wraps the MCP server and its dependencies.
type Server struct {
	srv     *server.MCPServer
	cfg     *Config
	sigCtx  context.Context
	sigStop context.CancelFunc
}

// New creates a new Server with the given config.
func New(cfg *Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		cfg:     cfg,
		sigCtx:  ctx,
		sigStop: cancel,
	}

	s.srv = server.NewMCPServer(
		"agentfs",
		"0.3.0",
		server.WithInstructions("agentfs MCP Server — native file system operations for AI agents"),
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)

	s.RegisterPing()
	return s
}

// RegisterPing registers a health-check ping tool.
func (s *Server) RegisterPing() {
	tool := mcp.NewTool("ping",
		mcp.WithDescription("Health check: respond with pong"))

	s.srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("pong"), nil
	})
}

// Start begins serving on the configured transport.
func (s *Server) Start() error {
	// Handle OS signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		s.sigStop()
	}()

	switch s.cfg.Transport {
	case "sse":
		return s.serveSSE()
	default:
		return s.serveStdio()
	}
}

func (s *Server) serveStdio() error {
	return server.ServeStdio(s.srv, server.WithStdioContextFunc(func(ctx context.Context) context.Context {
		return s.sigCtx
	}))
}

func (s *Server) serveSSE() error {
	sseServer := server.NewSSEServer(s.srv)
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	log.Printf("Starting SSE server on %s", addr)
	return sseServer.Start(addr)
}

// Server returns the underlying MCPServer for tool registration.
func (s *Server) Server() *server.MCPServer {
	return s.srv
}

// Config returns the server configuration.
func (s *Server) Config() *Config {
	return s.cfg
}

// Context returns the server context.
func (s *Server) Context() context.Context {
	return s.sigCtx
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() {
	s.sigStop()
}

// CallTool executes a single tool by name with JSON params.
// It finds the handler via ListTools and calls it directly.
func (s *Server) CallTool(ctx context.Context, name string, paramsJSON []byte) (string, error) {
	tools := s.srv.ListTools()
	t, ok := tools[name]
	if !ok {
		return "", fmt.Errorf("tool %q not found", name)
	}

	var args map[string]any
	if len(paramsJSON) > 0 {
		if err := json.Unmarshal(paramsJSON, &args); err != nil {
			return "", fmt.Errorf("invalid params JSON: %w", err)
		}
	} else {
		args = map[string]any{}
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}

	if ctx == nil {
		ctx = context.Background()
	}

	result, err := t.Handler(ctx, req)
	if err != nil {
		return "", err
	}

	// Extract text content
	var texts []string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			texts = append(texts, tc.Text)
		}
	}
	text := ""
	for i, t := range texts {
		if i > 0 {
			text += "\n"
		}
		text += t
	}

	if result.IsError {
		return "", fmt.Errorf("tool error: %s", text)
	}
	return text, nil
}
