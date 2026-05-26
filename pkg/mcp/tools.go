package mcp

import (
	"log"

	tools "github.com/agent-os/agent-os/pkg/mcp/tools"
)

// RegisterAll registers all MCP tools on the server.
// Each domain module registers its own tools.
func (s *Server) RegisterAll() {
	log.Println("Registering MCP tools...")

	// Module 0
	s.RegisterPing()

	// Module 1: file operations
	tools.RegisterFileOps(s.Server(), s.Config().ProjectRoot)

	// Module 2: context and search
	tools.RegisterContextTools(s.Server(), s.Config().ProjectRoot)
	tools.RegisterSearchTools(s.Server(), s.Config().ProjectRoot)

	// Module 3: session management
	tools.RegisterSessionTools(s.Server(), s.Config().SessionDir, s.Config().ProjectRoot)

	// Phase 6: skill management
	tools.RegisterSkillTools(s.Server())

	// Phase 7: provider management
	tools.RegisterProviderTools(s.Server())

	log.Printf("Server ready with %d tools", len(s.srv.ListTools()))
}
