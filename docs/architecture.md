# Architecture — Agent-Native OS

> 版本：v0.5.0
> 最终更新：2026-05-28

---

## 概述

**agent-os** 是一个面向 AI coding agent 的操作系统。它不是一个简单的工具集，而是从文件系统层到内核层的完整 OS 抽象——让 AI agent 可以在项目中真正「安家」。

核心思想：**通过 FUSE 虚拟文件系统，将所有 OS 能力暴露为文件操作**。Agent 只需要 `ls` 和 `cat`，就能发现项目、调度任务、管理记忆。

---

## 四层架构图

```
┌─────────────────────────────────────────────────────┐
│                  👤 Interface Layer                  │
│          CLI (agentfs) · MCP · Tauri GUI            │
├─────────────────────────────────────────────────────┤
│                  📡 MCP Protocol                     │
│           33 tools · Resources · Notify             │
├──────────────────┬──────────────────────────────────┤
│                  │           🧠 Kernel              │
│   🗂️ agentfs    │    调度 · 生命周期 · IPC · 路由    │
│   FUSE 文件系统  ├──────────────────────────────────┤
│                  │           👻 Daemon               │
│   13 虚拟目录    │    监控 · 定时 · 报告 · 挖掘       │
│   纯文件操作     ├──────────────────────────────────┤
│                  │          🏠 Workspace             │
│                  │   家目录 · 记忆 · 团队 · 沙箱      │
├──────────────────┴──────────────────────────────────┤
│                  🔌 Provider Layer                   │
│      Anthropic · OpenAI · DeepSeek · GLM · Ollama   │
└─────────────────────────────────────────────────────┘
```

从底向上看：

1. **Provider Layer** — LLM 提供商抽象，模型无关
2. **Workspace** — Agent 的家目录、记忆、团队协作空间、会话沙箱
3. **Daemon** — 后台守护进程，提供主动能力
4. **Kernel** — Agent 调度、生命周期管理、IPC 通信
5. **agentfs** — FUSE 虚拟文件系统，统一入口

---

## 各层详解

### 🗂️ Layer 1: agentfs — FUSE 虚拟文件系统

> 包：`pkg/fs/`

agent-os 的核心创新。通过 FUSE 将项目信息暴露为 13 个语义目录。

| 组件 | 职责 |
|------|------|
| `FuseFS` | FUSE 挂载点管理，路由文件操作到各虚拟目录 |
| `ContextNode` | `@context/` — 项目摘要、技术栈检测、统计 |
| `SearchNode` | `@search/` — 按符号/类型/依赖浏览代码 |
| `GraphNode` | `@graph/` — 依赖图谱、调用图（DOT 格式） |
| `RefactorNode` | `@refactor/` — 死代码检测、圈复杂度 |
| `TeamNode` | `@team/` — 多 Agent 身份管理、任务分配 |
| `TaskNode` | `@tasks/` — 任务看板、依赖 DAG |
| `SkillNode` | `@skills/` — 技能加载、激活、上下文注入 |
| `ProviderNode` | `@providers/` — LLM 提供商管理、API Key |
| `MemoryNode` | `@memory/` — 持久记忆读写 |
| `BridgeNode` | `@bridges/` — 飞书/Webhook 远程桥接 |
| `KernelNode` | `@kernel/` — 内核状态、Agent 管理 |
| `DaemonNode` | `@daemon/` — 守护进程控制 |
| `WorkspaceNode` | `@workspaces/` — 工作区管理、会话沙箱 |

**设计理念**：一切皆文件。Agent 只需 `cat @context` 就能理解项目，`cat @kernel/agents/a1b2c3/status` 就能查看其他 Agent 状态。

---

### 🏠 Layer 2: Workspace — Agent 工作区

> 包：`pkg/workspace/`, `pkg/sandbox/`

Agent 的「家」。每个 Agent 有独立的 home 目录、记忆系统和会话沙箱。

| 组件 | 职责 |
|------|------|
| `WorkspaceEngine` | 工作区创建、注册、持久化 |
| `HomeDir` | Agent 私有目录（cache、配置、profile） |
| `TeamSpace` | 团队共享空间，跨 Agent 协作 |
| `Memory` | 分类持久记忆（sessions / decisions / preferences / knowledge） |
| `SessionManager` | 会话沙箱——文件复制的隔离工作环境 |

**会话沙箱**（v0.5.0 新增）：

```
   create_session
   ┌──────────────────────────────────────────┐
   │                                          │
   │   原项目目录              会话副本          │
   │  ┌──────────┐         ┌──────────────┐   │
   │  │ src/     │  ────→  │ src/         │   │
   │  │ go.mod   │  copy   │ go.mod       │   │
   │  │ pkg/     │         │ pkg/         │   │
   │  └──────────┘         └──────┬───────┘   │
   │                               │           │
   │                        Agent 修改文件       │
   │                               │           │
   │                        ┌──────▼───────┐   │
   │                        │ src/ (已改)   │   │
   │                        │ go.mod (已改) │   │
   │                        └──────┬───────┘   │
   │                               │           │
   │                        session_diff        │
   │                        ┌──────▼───────┐   │
   │                        │ unified diff │   │
   │                        └──────┬───────┘   │
   │                               │           │
   │                   ┌───────────┴─────────┐  │
   │                   │                     │  │
   │            commit_session        discard_session
   │                   │                     │  │
   │            ┌──────▼──────┐       ┌──────▼──────┐
   │            │ 覆盖回原项目  │       │ 删除会话副本  │
   │            └─────────────┘       └─────────────┘
   │                                          │
   └──────────────────────────────────────────┘
```

特点：
- **无需 root**：纯文件复制，不依赖 OverlayFS
- **安全隔离**：修改只在副本中进行，不会影响原项目
- **原子提交**：commit 时一次性覆盖，保证一致性
- **完整 diff**：支持 unified diff 格式查看所有变更

---

### 👻 Layer 3: Daemon — 守护进程

> 包：`pkg/daemon/`

类比 Linux 的 PID 1 / systemd。提供 agent-os 的**主动能力**——不是被动等 agent 调用，而是主动发现和响应。

| 组件 | 类比 | 职责 |
|------|------|------|
| `Watcher` | inotify | 文件系统变更监控 + 事件回调 |
| `Scheduler` | cron | 定时任务引擎（YAML 配置） |
| `Reporter` | syslog | 定期报告生成 + 桥接推送 |
| `Miner` | 后台服务 | 潜意识循环：空闲时自动发现模式、优化建议 |

FUSE 入口：`@daemon/`

```
@daemon/
├── status           # daemon 运行状态
├── config           # daemon YAML 配置（可写）
├── schedule/        # 定时任务列表
├── watcher/         # 文件监控状态
├── reports/         # 已生成报告
├── miner/           # 潜意识挖掘状态
├── log              # daemon 日志
└── restart          # 写入重启 daemon
```

---

### 🧠 Layer 4: Kernel — Agent 内核

> 包：`pkg/kernel/`

类比 Linux Kernel。管理 Agent 的全生命周期。

| 组件 | 类比 | 职责 |
|------|------|------|
| `LifecycleManager` | 进程管理 | Agent 创建/运行/挂起/恢复/终止 |
| `Scheduler` | 调度器 | 任务提交 → 优先队列 → 依赖 DAG → 匹配 Agent |
| `ResourceManager` | 内存管理 | Token 配额、Agent 数量上限 |
| `IPC` | 进程间通信 | 频道订阅/发布、点对点消息、广播 |
| `ModelRouter` | 设备驱动 | 按任务类型/复杂度路由到不同 LLM |
| `ContextCache` | 页缓存 | LRU 缓存 + TTL 过期 |
| `StateStore` | 持久化 | JSONL 快照存储/恢复 |

FUSE 入口：`@kernel/`

```
@kernel/
├── agents/
│   ├── <agent-id>/
│   │   ├── status      # running / suspended / terminated
│   │   ├── type        # developer / architect / tester
│   │   ├── config      # JSON 配置
│   │   ├── resources   # 资源用量
│   │   ├── log         # 日志
│   │   ├── suspend     # 写入挂起
│   │   ├── resume      # 写入恢复
│   │   └── kill        # 写入终止
│   ├── spawn           # 写入 JSON 创建新 Agent
│   └── summary         # 所有 Agent 概览
├── tasks/
│   ├── submit          # 写入提交任务
│   ├── pending/running/completed/failed/
├── resources/
│   ├── usage           # 当前资源用量
│   └── limits          # 资源上限
├── channels/           # IPC 频道列表
└── config              # Kernel 配置
```

---

## 包结构

```
agent-os/
├── cmd/
│   ├── agentfs/              # CLI 入口（mount/unmount/init/skill/provider/session）
│   └── agentfs-mcp/          # MCP Server 入口
├── pkg/
│   ├── fs/                   # FUSE 文件系统核心（13 个虚拟目录节点）
│   ├── mcp/                  # MCP 协议层（33 个工具）
│   ├── context/              # 项目上下文引擎
│   ├── index/                # 语义索引（bleve + AST 分析）
│   ├── sandbox/              # 会话沙箱（文件复制模式）
│   ├── refactor/             # 重构引擎（死代码/复杂度）
│   ├── team/                 # 多 Agent 协同（身份/任务/通信/拓扑/编排）
│   ├── skill/                # 技能系统（Engine + Loader + 7 内置技能）
│   ├── provider/             # Provider 适配器（5 个实现）
│   ├── memory/               # 持久记忆引擎
│   ├── bridge/               # 远程桥接（飞书/Webhook）
│   ├── kernel/               # Agent Kernel（调度/生命周期/资源/IPC）
│   ├── daemon/               # Agent Daemon（Watcher/Scheduler/Reporter/Miner）
│   ├── workspace/            # Agent 工作区管理
│   └── plugin/               # 插件 SDK
├── gui/                      # Tauri 桌面 GUI
├── docs/                     # 用户文档
├── test/                     # 集成测试
├── plugins/                  # 内置插件
├── Makefile
└── *.md
```

---

## 数据流：AI Agent 如何发现和使用 agent-os

```
  AI Agent (Claude / GPT / DeepSeek)
         │
         │  1. 挂载 agentfs
         │     agentfs mount /project /project/.agentfs/mnt
         ▼
  ┌─────────────────────┐
  │   FUSE 虚拟文件系统    │
  │                      │
  │  2. 探索项目          │
  │     cat @context     │ → 技术栈、结构、文件统计
  │     ls  @search/     │ → 按符号浏览代码
  │     cat @graph/deps  │ → 依赖关系图
  │                      │
  │  3. 创建会话沙箱       │
  │     create_session   │ → 获得项目副本
  │                      │
  │  4. 隔离工作          │
  │     write_file (副本) │ → 在沙箱中修改文件
  │                      │
  │  5. 查看变更          │
  │     session_diff     │ → unified diff
  │                      │
  │  6. 提交或丢弃        │
  │     commit_session   │ → 变更回写原项目
  │     discard_session  │ → 丢弃变更
  └─────────────────────┘
         │
         │  7. 或通过 MCP 工具直接调用
         │     33 个工具，等效功能
         ▼
    任务完成 ✅
```

**关键路径**：Agent 首先通过 `@context` 理解项目全貌，然后在会话沙箱中安全地修改文件，最后选择提交或丢弃。全程无需 root 权限，无需特殊 API。

---

## 完整 FUSE 虚拟目录索引

| 虚拟目录 | Phase | 用途 |
|----------|-------|------|
| `@context` | P1 | 项目摘要（技术栈、结构、统计） |
| `@search/` | P3 | 语义浏览（按类型/符号/依赖/时间） |
| `@graph/` | P3 | 依赖图谱（DOT/JSON） |
| `@refactor/` | P3 | 重构工具（死代码/复杂度/lint） |
| `@team/` | P4 | 多 Agent 团队协作 |
| `@tasks/` | P4 | 任务看板 + DAG |
| `@skills/` | P6 | 技能系统（7 个内置技能） |
| `@providers/` | P7 | 模型提供商管理 |
| `@memory/` | P9 | 持久记忆（会话/决策/偏好/知识） |
| `@bridges/` | P9 | 远程桥接（飞书/Webhook） |
| `@kernel/` | P10 | Agent 内核管理 |
| `@daemon/` | P11 | 守护进程管理 |
| `@workspaces/` | P12 | Agent 工作区管理 |
