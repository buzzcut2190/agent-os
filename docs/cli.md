# CLI Reference

## `agentfs` — FUSE Filesystem Daemon

| Command | Description |
|---------|-------------|
| `agentfs init [dir]` | Initialize `.agentfs/` config in the target directory |
| `agentfs mount <src> <mnt>` | Mount the virtual filesystem |
| `agentfs mount <src> <mnt> --debug` | Mount with verbose FUSE logging |
| `agentfs mount <src> <mnt> --readonly` | Mount in read-only mode (disable writes) |
| `agentfs mount <src> <mnt> --allow-root` | Allow root to access the mount |
| `agentfs version` | Print version and build info |
| `agentfs plugins list` | List registered plugins |
| `agentfs plugins enable <name>` | Activate a plugin |
| `agentfs plugins disable <name>` | Deactivate a plugin |

### Mount Options

```
--debug        Enable FUSE debug output
--readonly     Disallow write operations
--allow-root   Permit root user access
--config PATH  Use a custom config file (default: .agentfs/config.yaml)
```

### Example

```bash
agentfs mount . .agentfs/mnt --debug --readonly
```

## `agentfs-mcp` — MCP Server

| Command | Description |
|---------|-------------|
| `agentfs-mcp` | Start MCP server on stdio (default) |
| `agentfs-mcp --transport sse` | Start with SSE transport on port 8080 |
| `agentfs-mcp --transport sse --port 9090` | SSE on custom port |
| `agentfs-mcp --tools PATH` | Load tools from a custom directory |
| `agentfs-mcp --verbose` | Enable debug-level logging |
| `agentfs-mcp version` | Print MCP server version |

### Transport Modes

- **stdio** (default): Standard input/output, used by Claude Desktop and other
  MCP clients that spawn the server as a subprocess.
- **sse**: Server-Sent Events over HTTP. Useful for remote agents or
  browser-based clients.

### Example

```bash
agentfs-mcp --transport sse --port 9000 --verbose
```
