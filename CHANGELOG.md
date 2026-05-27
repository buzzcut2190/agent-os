# Changelog

All notable changes to agent-os will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.5.0] - 2026-05-28

### Added

- **会话沙箱**：基于文件复制的轻量级会话系统，替代 OverlayFS 方案
  - 无需 root 权限，不依赖内核 OverlayFS
  - `create_session` / `commit_session` / `discard_session` 完整生命周期
  - `session_diff` 生成 unified diff 格式变更预览
- **CLI 会话管理**：`agentfs session` 子命令（create / list / diff / commit / discard）
- **MCP 会话工具**：6 个 MCP 会话工具（create_session, get_session, list_sessions, session_diff, commit_session, discard_session）
- **文档全面更新**：
  - 新增 `docs/architecture.md` 架构文档
  - 更新 `docs/getting-started.md`（会话工作流、Provider 配置、MCP 集成）
  - 新增 `CHANGELOG.md`
  - 更新 `README.md`（会话沙箱、Why agent-os、路线图、贡献指南）
- **CI/CD**：GitHub Actions 构建流水线

### Changed

- **沙箱方案迁移**：OverlayFS → 文件复制（消除 root 权限依赖）
- **GUI 错误处理优化**：改进 Tauri GUI 的错误展示和恢复流程

### Fixed

- FUSE 文件描述符相关问题修复

---

## [0.4.0] - 2026-05-26

### Added

- **Agent Kernel** (`pkg/kernel/`)：
  - LifecycleManager：Agent 创建/运行/挂起/恢复/终止
  - Scheduler：任务提交 → 优先队列 → 依赖 DAG → Agent 匹配
  - ResourceManager：Token 配额、Agent 数量上限
  - IPC：频道订阅/发布、点对点消息、广播
  - ModelRouter：按任务类型/复杂度路由 LLM
  - ContextCache：LRU 缓存 + TTL 过期
  - StateStore：JSONL 快照存储/恢复
- **Agent Daemon** (`pkg/daemon/`)：
  - Watcher：文件系统变更监控
  - Scheduler：YAML 定时任务
  - Reporter：定期报告生成 + 推送
  - Miner：潜意识循环（空闲模式发现）
- **Agent Workspace** (`pkg/workspace/`)：
  - 工作区创建/注册/持久化
  - Agent 家目录（home / scratch / artifacts）
  - 团队共享空间
  - 分类持久记忆（sessions / decisions / preferences / knowledge）
- **FUSE 目录扩展**：新增 `@kernel/`、`@daemon/`、`@workspaces/` 三个虚拟目录

### Fixed

- FUSE 文件描述符泄漏问题
- 并发挂载竞态条件修复
- 大文件读取内存溢出问题
- YAML 配置解析边界情况

---

## [0.3.0] - 2026-05-20

### Added

- **技能系统** (`pkg/skill/`)：
  - 7 个内置技能：code-review、test-generator、commit-message、auto-doc、architectural-review、dependency-audit、feedback-synthesis
  - 技能 Engine + Loader 框架
  - `agentfs skill` CLI 子命令
  - `@skills/` FUSE 虚拟目录
- **Provider 系统** (`pkg/provider/`)：
  - 5 个 LLM 提供商适配器：Anthropic、OpenAI、DeepSeek、GLM、Ollama
  - API Key 加密存储
  - `agentfs provider` CLI 子命令
  - `@providers/` FUSE 虚拟目录
- **Tauri GUI** (`gui/`)：桌面应用界面
- **持久记忆** (`pkg/memory/`)：分类记忆系统（会话/决策/偏好/知识）
- **远程桥接** (`pkg/bridge/`)：飞书和 Webhook 集成

---

## [0.2.0] - 2026-05-15

### Added

- **MCP 协议层** (`pkg/mcp/`)：
  - 基于 mcp-go 的 MCP Server 实现
  - stdio 和 SSE 两种传输模式
  - 22+ MCP 工具
  - `agentfs-mcp` 独立二进制
- **语义层**：
  - `@search/`：按函数/类/符号/类型浏览代码
  - `@graph/`：依赖图谱、调用图（DOT 格式）
  - `@refactor/`：死代码检测、圈复杂度分析
- **Agent 协作** (`pkg/team/`)：
  - 多 Agent 身份管理
  - 任务分配和拓扑编排
  - `@team/` 虚拟目录
- **Phase 1-5 生产化**：
  - P1: 项目上下文（`@context`）
  - P2: 文件操作（读写/搜索/移动/删除）
  - P3: 语义索引
  - P4: 多 Agent 团队
  - P5: 系统稳定性和测试

---

## [0.1.0] - 2026-05-10

### Added

- 项目初始脚手架
- FUSE 文件系统基础框架
- CLI 入口（`cmd/agentfs/`）
- 基础构建系统（Makefile）
- `.agentfs/config.yaml` 配置管理
- `agentfs init` / `agentfs mount` / `agentfs unmount` 命令
