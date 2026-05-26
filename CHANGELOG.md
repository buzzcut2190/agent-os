# Changelog

## v0.4.0 (2026-05-27) — Agent-Native OS 战略升级

> 从"给 AI agent 用的文件系统"升级为 **"AI agent 可以在其中安家的操作系统"**

### Phase 10: Agent Kernel 🧠（新增）
- LifecycleManager: Agent 全生命周期（create → run → suspend → resume → kill）
- Scheduler: 优先级队列 + 依赖DAG + 轮询调度循环
- ResourceManager: Token/Agent 配额管理 + 按类型限制
- IPC: 频道订阅/发布 + 点对点消息 + 广播
- ModelRouter: 按任务类型/复杂度路由到不同 LLM
- ContextCache: LRU 缓存 + TTL 过期
- StateStore: JSONL 快照存储/恢复
- WaitGroup: Agent 协同等待原语
- `@kernel/` FUSE 虚拟目录（agents/tasks/resources/channels/spawn/config/restart）

### Phase 11: Agent Daemon 👻（新增）
- FileWatcher: 文件系统变更监控
- ScheduleEngine: 定时任务引擎（YAML 配置）
- Reporter: 定期报告生成 + 桥接推送
- BackgroundMiner: 潜意识循环（空闲时自动发现模式/优化）
- PID 文件管理 + 优雅启停
- `@daemon/` FUSE 虚拟目录（status/config/schedule/watcher/reports/miner/log/restart）

### Phase 12: Agent Workspace 🏠（新增）

### Bug Fixes

- **🔒 FUSE 自递归死锁：** `context.NewEngine` 的 `generate()` 用 `filepath.WalkDir` 遍历项目目录时未排除 `.agentfs/` 挂载点，导致递归 FUSE 请求 → 进程 D 状态卡死。修复：将 `.agentfs/` 加入 Exclude 列表。
- **💀 虚拟文件 EBADF：** `FlushFile()` 对 nil `*os.File` 句柄返回 `syscall.EBADF`（错误的文件描述符）。修复：改为返回 nil，与 `ReleaseFileHandle()` 一致。
- **🚫 Provider Ping nil Context：** `cmd/agentfs/provider.go` 中三处 `p.Ping(nil)` 导致 `net/http: nil Context`。修复：改为 `p.Ping(context.Background())`。
- **🔑 KeyStore 未接入 Provider：** `provider key` 命令将 API Key 存入 keystore，但 `runProvider` 中用 `_ = key` 丢弃，未应用到 provider 实例。修复：`Provider` 接口新增 `SetAPIKey(key string)` 方法，key-sync 循环中调用 `p.SetAPIKey(key)`。
- Engine: 工作区创建/注册/持久化（registry.json）
- Home: Agent 家目录（cache + 私有配置）
- Team: 团队共享空间
- Memory: 分门别类的持久记忆（sessions/decisions/preferences/knowledge）
- WorkArea: 暂存区 + artifacts 产出物
- AgentProfile: Agent 个性化配置
- `@workspaces/` FUSE 虚拟目录（create/search/stats/me）

## v0.3.0 (2026-05-25) — 技能/提供商/GUI/记忆桥接

### Phase 6: Skills 原生化
- Skill 引擎（Engine/Loader/Builtin/Config）
- `@skills/` FUSE 虚拟目录
- 7 个内置技能：
  - code-review: 自动化代码审查
  - test-generator: 自动生成测试用例
  - commit-message: Git 提交信息生成
  - auto-doc: 自动文档生成
  - architectural-review: 架构审查
  - dependency-audit: 依赖审计
  - feedback-synthesis: 反馈综合
- MCP 工具扩展 + CLI 子命令

### Phase 7: Provider Adapter
- Provider 抽象接口（Chat/Ping/List）
- 5 个 Provider 实现：Anthropic、OpenAI、DeepSeek、GLM（智谱）、Ollama
- KeyStore 安全存储
- ModelRouter 路由策略（priority/latency/random）
- `@providers/` FUSE 虚拟目录
- MCP 工具 + CLI 交互式初始化向导

### Phase 8: Desktop GUI
- Tauri 2.x 项目骨架（React + TypeScript + Vite）
- shadcn/ui + Tailwind CSS
- 虚拟目录浏览器
- 实时状态面板

### Phase 9: 持久记忆 & 桥接
- Memory 存储引擎（JSONL + 检索）
- `@memory/` FUSE 虚拟目录（sessions/decisions/preferences/knowledge/search/note/stats）
- 自动记忆采集（会话摘要、决策提取、偏好学习）
- Bridge 桥接引擎 + Registry
- 飞书桥接完整实现
- Webhook 桥接
- `@bridges/` FUSE 虚拟目录（status/register）
- MCP 工具 + CLI 子命令

## v0.2.0 (2026-05-24) — MCP/语义/Agent协作/生产化

- Phase 1: FUSE filesystem with @context, session sandbox
- Phase 2: MCP Server with 22+ tools
- Phase 3: Semantic layer with @search, @graph, @refactor
- Phase 4: Multi-agent coordination with @team, @tasks
- Phase 5: Cross-platform builds, caching, plugin SDK, documentation

## v0.1.0 (2026-05-22) — 初始版本

- Initial scaffold with go.mod, Makefile, and project structure
- FUSE filesystem prototype with virtual node framework
- MCP server skeleton with tool registration infrastructure
