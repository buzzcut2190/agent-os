# agent-os — Agent-Native Operating System

> 一个 **AI agent 可以在其中「安家」的操作系统**。  
> 不是"给 AI agent 用的文件系统"，而是从文件系统到内核层、守护进程层、工作空间层完整重构的操作系统。

[![Go Version](https://img.shields.io/badge/Go-1.25-blue)](go.mod)
[![Version](https://img.shields.io/badge/version-v0.4.0-green)](CHANGELOG.md)

**agent-os** 让 AI coding agent（Claude Code、Cursor、Codex 等）获得「原生 OS 体验」——通过 FUSE 文件系统暴露 13 个语义虚拟目录，外加 MCP 协议、SKill 系统、Agent 内核调度、守护进程、工作空间管理。

---

## 完整能力栈

| 层 | 入口 | 说明 |
|----|------|------|
| 🧠 **Agent Kernel** | `@kernel/` | 调度 Agent 进程、管理生命周期、分配资源、IPC 通信 |
| 👻 **Agent Daemon** | `@daemon/` | 常驻守护进程：文件监控、定时任务、报告生成、潜意识挖掘 |
| 🏠 **Agent Workspace** | `@workspaces/` | 每个 Agent 的家目录、团队共享空间、产出物管理 |
| 🗂️ **agentfs** | 挂载点 | 13 个虚拟目录 + 22+ MCP 工具 |

## Features

| 虚拟目录 | 说明 |
|----------|------|
| `@context` | 自动生成的项目摘要（技术栈、结构、统计） |
| `@search/` | 按类型/函数/类/符号浏览代码 |
| `@graph/` | 依赖图谱、调用图、导入关系（DOT 格式） |
| `@refactor/` | 死代码检测、圈复杂度、lint |
| `@team/` | 多 Agent 团队协作空间 |
| `@tasks/` | 任务看板 + 依赖 DAG |
| `@skills/` | 7 个内置技能（代码审查/测试生成/文档/提交信息/审计等） |
| `@providers/` | 5 个 LLM 提供商（Anthropic/OpenAI/DeepSeek/GLM/Ollama） |
| `@memory/` | 持久记忆（会话/决策/偏好/知识） |
| `@bridges/` | 远程桥接（飞书/Webhook） |
| `@kernel/` | Agent 内核状态（进程/任务/资源/IPC） |
| `@daemon/` | 守护进程控制（监控/定时/报告/挖掘） |
| `@workspaces/` | Agent 工作区管理 |

## Quick Start

```bash
# 构建
make build

# 初始化项目
agentfs init

# 挂载虚拟文件系统
agentfs mount . .agentfs/mnt

# 浏览项目摘要
cat .agentfs/mnt/@context

# 启动 MCP Server（供 AI agent 调用）
agentfs-mcp
```

## Documentation

- [Architecture](ARCHITECTURE.md) — 完整架构说明
- [Roadmap](ROADMAP.md) — 全阶段路线图
- [Getting Started](docs/getting-started.md) — 快速入门
- [CLI Reference](docs/cli.md) — 命令参考
- [MCP Tools](docs/mcp-tools.md) — MCP 工具参考

## Architecture

```
                        🧠 Agent Kernel (P10)
                           调度 · 生命周期 · 资源 · IPC
                        👻 Agent Daemon (P11)
                           监控 · 定时 · 报告 · 挖掘
                        🏠 Agent Workspace (P12)
                           家目录 · 团队空间 · 记忆
                        🗂️ agentfs (P1-P9)
                           13 虚拟目录 · 22+ MCP 工具
```

详细架构图见 [`ARCHITECTURE.md`](ARCHITECTURE.md)。

## Version History

| 版本 | 日期 | 内容 |
|------|------|------|
| v0.4.0 | 2026-05-26 | 🧠👻🏠 Kernel + Daemon + Workspace（OS 战略升级）|
| v0.3.0 | 2026-05-25 | Skills + Provider + GUI + 记忆/桥接 |
| v0.2.0 | 2026-05-24 | MCP + 语义层 + Agent 协作 + 生产化 |
| v0.1.0 | 2026-05-22 | 初始原型 |

## Troubleshooting

### 1. 挂载失败：`permission denied`

如果在 `.agentfs/mnt` 上挂载时遇到 `permission denied`，通常是**有残留的僵尸挂载点**（上次 `agentfs mount` 进程被杀死后，FUSE 挂载还留在内核里）。

```bash
# 先卸载
fusermount -uz .agentfs/mnt

# 确认无残留
mount | grep agentfs  # 应该无输出

# 重新挂载
agentfs mount . .agentfs/mnt
```

### 2. 挂载后 `cat .agentfs/mnt/@context` 卡死（D 状态）

这是已知的**自递归死锁**问题，已修复。原因：`context.NewEngine` 的 `generate()` 用 `filepath.WalkDir` 遍历项目目录，会钻进 FUSE 挂载点 `.agentfs/` 内部，导致 FUSE 请求发回自身进程——进程陷入不可杀的 D 状态。

**修复：** `.agentfs/` 已加入 Exclude 列表，被 `WalkDir` 自动跳过。

如果之前遇到过此问题，残留的 D 状态进程需要重启系统才能彻底清除。

### 3. `cat .agentfs/mnt/README.md` 返回 `错误的文件描述符`

修复于 `v0.4.0` 开发分支。原因：虚拟文件（从 `@context/`、`@kernel/` 等动态生成的文件）的 `FlushFile()` 对 nil 的 `*os.File` 句柄返回了 `syscall.EBADF`（错误文件描述符）。改为返回 nil，与 `ReleaseFileHandle()` 行为一致。

### 4. Provider 配置流程

分两步：

```bash
# 1. 交互式初始化（写 YAML 配置，不存 API Key）
agentfs provider init

# 2. 单独存入 API Key（加密存到 keystore，不落明文）
agentfs provider key <provider-name>
# 然后粘贴 key（不回显）

# 验证
agentfs provider list
# → 输出应显示 HEALTHY: ok
```

> ⚠️ YAML 配置文件中字段是 snake_case（`base_url`、`api_key`），不是 camelCase。`agentfs provider init` 会自动生成正确的格式。

### 5. MCP Server 用法

```bash
# stdio 模式（供 Claude Code 等本地 agent 使用）
agentfs-mcp serve --transport stdio

# SSE 模式（HTTP 端口 9090）
agentfs-mcp serve --transport sse --port 9090
```

MCP 服务器会提供 33 个工具，包括文件操作、代码搜索、技能管理、提供商配置等。

## License

MIT
