# Getting Started

## Prerequisites

- **Go 1.25+**（项目使用 Go 1.25.5）
- **Linux** with FUSE support:
  - Kernel module loaded (`/dev/fuse` accessible)
  - `libfuse` development headers: `apt install libfuse-dev` (Debian/Ubuntu)
- **无需 root 权限**（会话沙箱使用文件复制，不依赖 OverlayFS）

## Installation

```bash
# Clone and build from source
git clone https://github.com/agent-os/agent-os
cd agent-os
make build
```

Binaries are written to `./bin/agentfs` and `./bin/agentfs-mcp`.

Optionally install to system PATH:

```bash
make install
# installs to /usr/local/bin/agentfs and /usr/local/bin/agentfs-mcp
```

## Initialize a Project

Run `agentfs init` in your project root to create the configuration:

```bash
cd my-project
agentfs init
```

This writes `.agentfs/config.yaml` with sensible defaults for language detection
and index settings.

## Mount the Virtual Filesystem

```bash
agentfs mount . .agentfs/mnt
```

Leave the mount running in the foreground. In another terminal, explore:

```bash
cat .agentfs/mnt/@context          # project summary
ls .agentfs/mnt/@search/           # semantic index
cat .agentfs/mnt/@graph/deps.dot   # dependency graph
cat .agentfs/mnt/@refactor/dead    # dead code report
```

## Session Workflow

Sessions provide isolated sandboxes for AI agents to work safely. Changes are made
on a copy of the project, then committed or discarded.

### CLI Example

```bash
# Create a new session (copies project files to .agentfs/sessions/<id>/)
agentfs session create

# List active sessions
agentfs session list

# Work inside the session (modify files in the session copy)
# The agent works in .agentfs/sessions/<id>/...

# View changes (unified diff between session and original)
agentfs session diff <session-id>

# Commit: overwrite original project with session changes
agentfs session commit <session-id>

# Or discard: delete the session copy
agentfs session discard <session-id>
```

### MCP Tool Example

AI agents using MCP can call session tools directly:

```
# Create a session → returns session ID and path
create_session(project_path="/home/user/my-project")

# Modify files in the session copy
write_file(path="/home/user/my-project/.agentfs/sessions/<id>/src/main.go", content="...")

# Check what changed
session_diff(session_id="<id>")

# Commit when satisfied
commit_session(session_id="<id>")

# Or throw away
discard_session(session_id="<id>")
```

## Unmount

```bash
fusermount -u .agentfs/mnt
```

## Provider Configuration

agent-os supports 5 LLM providers: Anthropic, OpenAI, DeepSeek, GLM (Zhipu), Ollama.

### Interactive Setup

```bash
# Launch interactive configuration wizard
agentfs provider init
```

### Manual Setup

```bash
# Set API key for a provider (stored encrypted)
agentfs provider key deepseek
# → Enter API key: sk-xxx...

# Verify configuration
agentfs provider list
# → NAME      TYPE              MODELS                               STATUS
# → deepseek  openai-compatible deepseek-chat,deepseek-v4-pro,...    ok

# Switch default model
agentfs provider model deepseek-chat

# Test connectivity
agentfs provider test deepseek
# → ✅ Connected: deepseek-chat (latency: 120ms)
```

### Example: Configure Ollama (Local)

```bash
# Ollama needs no API key, just point to the local server
agentfs provider key ollama
# → Enter API key (press Enter for empty): 

# Verify
agentfs provider list
# → ollama   ollama-native  llama3,deepseek-coder-v2,...   ok
```

## MCP Server Setup

### For Claude Desktop

Add to Claude Desktop config (`~/.config/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "agent-os": {
      "command": "/usr/local/bin/agentfs-mcp",
      "args": ["serve", "--transport", "stdio"],
      "cwd": "/path/to/your/project"
    }
  }
}
```

Restart Claude Desktop. The agent-os tools will appear in the tools panel.

### For Cline (VS Code)

1. Install the Cline extension in VS Code
2. Open Cline settings → MCP Servers
3. Add a new server:

```
Name: agent-os
Command: /usr/local/bin/agentfs-mcp
Args: serve --transport stdio
Working Dir: /path/to/your/project
```

4. Cline will now have access to all 33 agent-os MCP tools

### For Custom Integration

```bash
# Start as SSE server for HTTP-based clients
agentfs-mcp serve --transport sse --port 8080

# Or use stdio for piped communication
agentfs-mcp serve --transport stdio
```

## Skill Usage

```bash
# List available skills
agentfs skill list

# Activate a skill
agentfs skill activate code-review

# Deactivate
agentfs skill deactivate code-review

# Get skill context (for prompt injection)
agentfs skill context code-review
```

## Next Steps

- Read the [Architecture](architecture.md) doc for system design details
- Check the [CLI Reference](cli.md) for all commands and flags
- See [MCP Tools](mcp-tools.md) for the full tool catalog
- Read [CHANGELOG](../CHANGELOG.md) for version history
