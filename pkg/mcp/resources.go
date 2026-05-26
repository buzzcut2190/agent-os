package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// RegisterResources registers MCP resource URIs on the server.
func (s *Server) RegisterResources() {
	// context://project/summary
	tmpl := mcp.NewResourceTemplate(
		"context://project/summary",
		"Project Summary",
		mcp.WithTemplateDescription("Full project summary in markdown"),
		mcp.WithTemplateMIMEType("text/markdown"),
	)
	s.srv.AddResourceTemplate(tmpl, func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return nil, fmt.Errorf("not implemented")
	})
}
