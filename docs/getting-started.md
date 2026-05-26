# Getting Started

## Prerequisites

- Go 1.25+
- Linux with FUSE support (kernel module loaded, `/dev/fuse` accessible)
- `libfuse` development headers (`apt install libfuse-dev` on Debian/Ubuntu)

## Installation

```bash
# Clone and build from source
git clone https://github.com/agent-os/agent-os
cd agent-os
make build
```

Binaries are written to `./bin/agentfs` and `./bin/agentfs-mcp`.

Optionally install to `$GOPATH/bin`:

```bash
make install
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

## Unmount

```bash
fusermount -u .agentfs/mnt
```

## Next Steps

- Run `agentfs-mcp` to expose tools for your AI agent via MCP
- Read the [CLI Reference](cli.md) for all commands and flags
- Check [MCP Tools](mcp-tools.md) to see the full tool catalog
