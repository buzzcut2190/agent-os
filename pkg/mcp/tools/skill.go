package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agent-os/agent-os/pkg/skill"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// EnsureEngine initializes the global skill engine if not already set.
var globalSkillEngine *skill.Engine

func getEngine() *skill.Engine {
	if globalSkillEngine == nil {
		globalSkillEngine = skill.NewEngine("", "")
		globalSkillEngine.LoadAll()
	}
	return globalSkillEngine
}

// SetSkillEngine allows the caller to inject a pre-configured engine.
func SetSkillEngine(eng *skill.Engine) {
	globalSkillEngine = eng
}

// RegisterSkillTools registers all skill-related MCP tools.
func RegisterSkillTools(srv *server.MCPServer) {
	registerListSkills(srv)
	registerActivateSkill(srv)
	registerDeactivateSkill(srv)
	registerGetSkillContext(srv)
}

// list_skills
func registerListSkills(srv *server.MCPServer) {
	tool := mcp.NewTool("list_skills",
		mcp.WithDescription("List all available skills. Optionally filter by state ('active', 'inactive') or by tag."),
		mcp.WithString("filter", mcp.Description("Optional filter: 'active', 'inactive', or a tag name.")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		eng := getEngine()
		filter := req.GetString("filter", "")

		var skills []skill.SkillInfo
		switch filter {
		case "active":
			skills = eng.Active()
		case "inactive":
			all := eng.List()
			activeNames := make(map[string]bool)
			for _, a := range eng.Active() {
				activeNames[a.Name] = true
			}
			for _, s := range all {
				if !activeNames[s.Name] {
					skills = append(skills, s)
				}
			}
		default:
			if filter != "" {
				// Filter by tag.
				all := eng.List()
				for _, s := range all {
					for _, t := range s.Tags {
						if t == filter {
							skills = append(skills, s)
							break
						}
					}
				}
			} else {
				skills = eng.List()
			}
		}

		data, err := json.MarshalIndent(skills, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("marshal skills: %v", err)), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})
}

// activate_skill
func registerActivateSkill(srv *server.MCPServer) {
	tool := mcp.NewTool("activate_skill",
		mcp.WithDescription("Activate a skill by name. Idempotent — activating an already-active skill succeeds."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Skill name to activate.")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}
		eng := getEngine()
		if err := eng.Activate(name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("activate skill %q: %v", name, err)), nil
		}
		eng.SaveState()
		return mcp.NewToolResultText(fmt.Sprintf("skill %q activated", name)), nil
	})
}

// deactivate_skill
func registerDeactivateSkill(srv *server.MCPServer) {
	tool := mcp.NewTool("deactivate_skill",
		mcp.WithDescription("Deactivate a skill by name. Idempotent."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Skill name to deactivate.")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}
		eng := getEngine()
		if err := eng.Deactivate(name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("deactivate skill %q: %v", name, err)), nil
		}
		eng.SaveState()
		return mcp.NewToolResultText(fmt.Sprintf("skill %q deactivated", name)), nil
	})
}

// get_skill_context
func registerGetSkillContext(srv *server.MCPServer) {
	tool := mcp.NewTool("get_skill_context",
		mcp.WithDescription("Get the rendered context.md for a skill. Use this to inject skill context into agent prompts."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Skill name.")),
	)
	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("name is required"), nil
		}
		eng := getEngine()
		content, err := eng.GetContext(name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get context for %q: %v", name, err)), nil
		}
		return mcp.NewToolResultText(content), nil
	})
}
