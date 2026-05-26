package mcp

import (
	"context"
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
