# MCP Tools Reference

The `agentfs-mcp` server exposes 22+ tools for AI agents via the
Model Context Protocol. Tools are grouped by category below.

## Context Tools

| Tool | Parameters | Description |
|------|-----------|-------------|
| `get_context` | `path` (optional) | Return the `@context` summary for the project or subdirectory |
| `get_file_summary` | `path` | Summarize a single file (language, LOC, symbols) |

## Search Tools

| Tool | Parameters | Description |
|------|-----------|-------------|
| `search_code` | `query`, `language`, `type` | Full-text search across the semantic index |
| `list_symbols` | `path`, `kind` | List symbols (func/type/class) in a file or directory |
| `find_references` | `symbol`, `path` | Find all references to a symbol |
| `find_definition` | `symbol` | Jump to a symbol's definition |

## Graph Tools

| Tool | Parameters | Description |
|------|-----------|-------------|
| `dependency_graph` | `format` (dot/json) | Return the full project dependency graph |
| `call_graph` | `function` | Show callers and callees of a function |
| `import_map` | `path` | Show imports for a file or package |

## Refactor Tools

| Tool | Parameters | Description |
|------|-----------|-------------|
| `detect_dead_code` | `path` (optional) | Find unreachable functions and variables |
| `cyclomatic_complexity` | `path` (optional) | Compute complexity scores per function |
| `lint` | `path` (optional) | Run linter and return diagnostics |

## File Tools

| Tool | Parameters | Description |
|------|-----------|-------------|
| `read_file` | `path` | Read a file from the workspace |
| `write_file` | `path`, `content` | Write content to a file |
| `list_dir` | `path` | List directory contents |
| `delete_file` | `path` | Delete a file or directory |

## Session Tools

| Tool | Parameters | Description |
|------|-----------|-------------|
| `session_create` | `name` | Create a new isolated session (OverlayFS) |
| `session_commit` | `session_id` | Persist session changes to the base workspace |
| `session_discard` | `session_id` | Discard session and free sandbox |
| `session_list` | — | List active sessions |

## Team Tools

| Tool | Parameters | Description |
|------|-----------|-------------|
| `team_status` | — | List agents, roles, and workspace assignments |
| `team_message` | `agent_id`, `message` | Send a message to another agent |
| `team_shared_context` | — | View the team-wide shared context |
