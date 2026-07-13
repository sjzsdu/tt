# Formula 中可复用的并行编排、调度与多 run 研究入口

> 面向 bead `tt-0gu`
> 上游依赖：无
> 会阻塞：`tt-2tr`（多 formula 运行能力的最小验证场景矩阵）、`tt-4ce`（MVP 方向与阶段边界）

这份文档的目的不是重新设计 formula 系统，而是把仓库里**已经存在**、且与后续“多 formula / 多 run”讨论最接近的拼图集中列出来，帮助后续 bead 直接从现有实现与测试起步。

## 先说边界：当前 bead 只解决什么

`tt-0gu` 本身不实现多 formula 运行能力，也不新增 runtime 行为。它只做两件事：

1. 说明当前仓库里哪些 pattern 已经存在、可直接复用；
2. 说明这些 pattern 的边界在哪里，避免把已有能力误当成缺失能力。

尤其要先分清：

- **单个 run 内的 loop / foreach 并发**：已经存在。
- **多个顶层 formula run 的统一调度、统一观察、统一交互归属**：目前没有现成监控平面，仍是后续 bead 要解决的缺口。

可先搭配阅读：

- `docs/formula/current-run-model-and-top-level-concurrency-boundaries.md`
- `docs/formula/run-store-and-web-dashboard-gaps-for-multi-run-monitoring.md`

## 模式 1：loop 并发 / foreach 并发

### 现有入口

- builtin formulas
  - `san-sheng-liu-bu`
  - `fresh-topic-docs`
  - `keep-coding`
- 参考文档
  - `ai-docs/step-kinds-reference.md` 的 `loop` 段落
- 关键测试
  - `cmd/formula/formula_runtime_test.go`
    - loop iteration session 隔离
    - resume 时 loop ancestor exclusion

### 已有能力能复用什么

- 在**单个 run 内**按数组 / foreach 展开多次迭代。
- `parallel = true` + `max_concurrency` 控制并发度。
- 每个 iteration 有独立 node id / 上下文路径，可单独收集结果。
- agent session 已有 iteration 级隔离测试，避免多个 iteration 共享同一会话污染上下文。
- resume 逻辑已经认识 loop 祖先：失败恢复时不会天真地把 loop body 当成普通单步重用。

### 不能覆盖什么

- loop 并发只是**一个 run 内部**的 fan-out/fan-in。
- 它不能直接代表“同时运行多个顶层 formula”的生命周期管理。
- 也不能直接提供多 run 的统一列表、批次状态、统一取消 / 重试、统一 human input 归口。

### 后续 bead 怎么用

- `tt-2tr` 可把 loop 并发作为“已有相邻能力”的基线场景：证明系统已经能在一个 run 里并发处理多任务。
- `tt-4ce` 需要明确写出：未来 MVP 若涉及多 formula 运行，不应把 loop 并发误写成顶层多 run 能力已存在。

## 模式 2：schedule 触发

### 现有入口

- CLI：`tt formula schedule`
- 测试：`cmd/formula/formula_schedule_test.go`
- 帮助输出：`cmd/formula/formula_help_test.go`

### 已有能力能复用什么

- `every` 间隔触发。
- cron 表达式触发。
- `run-now` / `max-runs` 等调度约束。
- “下一次 tick 如何计算”已经有独立测试夹具。

### 不能覆盖什么

- schedule 决定的是“何时启动下一次 run”。
- 它不提供多 run 聚合视图，也不描述多个已启动 run 之间的关系。
- 它也不回答“同一个计划触发的多个 run 是否属于一组、如何查看整体进度”。

### 后续 bead 怎么用

- 可把 schedule 当成未来多 run 场景的**触发来源**之一。
- 但若要定义 MVP，必须额外引入“run group / batch / supervisor”之类聚合概念，不能只复用 schedule 本身。

## 模式 3：run reopen / dashboard reopen / 单 run 持久化观察

### 现有入口

- 文档：`ai-docs/overview.md`、`ai-docs/module-map.md`
- CLI：`tt formula runs`、`tt formula run open`、`tt formula run resume`
- 测试：
  - `cmd/formula/formula_dashboard_updates_test.go`
  - `cmd/formula/formula_runtime_test.go`
  - `internal/formula/runtime/formularun_store_test.go`

### 已有能力能复用什么

- run store 已经把单次运行保存为可 reopen 的目录与状态。
- dashboard 可根据 snapshot、timeline、repairs、workspace 等信息恢复单 run 视图。
- reopen 语义已覆盖：
  - timeline logs
  - workspace ready 状态
  - final report chat
  - waiting / completed / failed 等状态映射

### 不能覆盖什么

- reopen 是围绕**某一个 run id**展开的。
- 目前没有“打开一组 run 的总览页”这种聚合入口。
- dashboard state 结构天然偏单 run，不是批处理控制台。

### 后续 bead 怎么用

- 后续场景矩阵可直接把单 run reopen 当作对照组。
- MVP 边界讨论应明确：哪些页面/API 可以扩展，哪些需要新的多 run 聚合层。

## 模式 4：worktree 隔离工作区

### 现有入口

- builtin formulas
  - `gongbu`
  - `github-pr-rebase-main`
- runtime
  - `internal/formula/runtime/workspace.go`
  - `internal/formula/runtime/environment.go`
- 测试
  - `cmd/formula/formula_dashboard_paths_test.go`
  - `cmd/formula/formula_runtime_test.go`
  - `internal/formula/runtime/executor_test.go`

### 已有能力能复用什么

- run 级 worktree 创建与 cleanup。
- agent / script 能在隔离工作区内执行。
- runtime 已有 workspace-aware prompt/session guard，降低误操作原仓库的风险。
- dashboard 已能展示当前 run 使用的 workspace 路径。

### 不能覆盖什么

- 当前 worktree 主要服务于“一个 run 对应一个隔离工作区”的模式。
- 它不天然包含多 run 的工作区池化、批量清理、跨 run 资源配额等能力。
- 也没有聚合视图告诉你“这一批 run 一共创建了哪些 worktrees”。

### 后续 bead 怎么用

- 可以把 worktree 当成多 run 能力里的“执行隔离基础设施”，不是“调度/监控平面”。
- 若后续要支持多 run 并发，worktree 管理很可能是实现层复用点，而不是产品层入口。

## 模式 5：resume / human input

### 现有入口

- CLI
  - `tt formula run resume`
  - `tt formula run input`
- 测试
  - `cmd/formula/formula_human_input_test.go`
  - `cmd/formula/formula_runtime_test.go`
  - `internal/formula/runtime/formularun_store_test.go`
- 文档
  - `ai-docs/step-kinds-reference.md`

### 已有能力能复用什么

- run 可进入 waiting / human_input_required。
- human input request 会落盘，可按 step id 定位并提交输入。
- 提交输入后可继续 resume，并保留已有上下文。
- runtime 已考虑 resume 时哪些依赖可以复用、哪些需要重跑。

### 不能覆盖什么

- 这套机制围绕**单 run + 单 snapshot**设计。
- 它没有回答“多个 run 同时等待人类输入时如何统一归并、排序、授权、批量处理”。
- 也没有“某个交互任务属于哪个 run group / batch”的概念。

### 后续 bead 怎么用

- `tt-2tr` 应把“单 run human input + resume”列为必须保留的兼容场景。
- `tt-4ce` 需要明确：如果做多 run 监控，interaction ownership / assignment 是一个新增维度，不是现成能力。

## 模式 6：嵌套 formula / builtin 复用

### 现有入口

- `github-pr-fix-comments` → 复用 `bug-fix`
- `keep-coding` → 循环调用 `bead-coding`
- `bead-coding` → workflow 层单 bead 执行器：读取 beads 上下文，内嵌无交互 `coding` 完成实现与验证，再负责提交和关闭
- atomics
  - `run-validation`
  - `github-fetch-pr`
  - `github-list-my-prs`

### 已有能力能复用什么

- 证明当前系统已经有“一个公式调用另一个公式”的真实工作流。
- builtin 与 atomics 已提供可落地的拼装方式，而不是纸面设计。
- 很适合作为后续 bead 的真实样例来源，尤其是“父公式协调子任务”的叙事结构。

### 不能覆盖什么

- 公式复用 ≠ 多 run 聚合控制面。
- 即便一个父公式内部触发多个子工作，也不等价于当前产品已经有统一多 run 监控能力。

## 最值得先看的测试夹具

### `cmd/formula/formula_runtime_test.go`

重点看：

- workspace request 如何覆盖默认 workspace
- workspace guard 如何注入 agent prompt
- loop iteration session 如何隔离
- dashboard event sink / snapshot 映射
- resume dependency exclusions 如何处理 loop ancestor

这是后续理解“现有行为已经被什么测试锁住”的最好入口之一。

### `cmd/formula/formula_schedule_test.go`

重点看：

- `every` 与 `cron` 的解析
- mutually exclusive source 校验
- next tick 计算
- `max-runs` 行为

适合作为“已有 schedule 能力边界”的最小证据。

### `cmd/formula/formula_human_input_test.go`

重点看：

- human input 请求与提交的 CLI 路径
- 字段校验与错误行为

适合作为“需要保留的单 run 交互路径”证据。

### `cmd/formula/formula_dashboard_updates_test.go`

重点看：

- timeline log 更新
- workspace ready 反映到 snapshot
- final report chat 生命周期
- repair 确认闭环

适合作为 dashboard reopen / state 恢复入口。

### `internal/formula/runtime/formularun_store_test.go`

重点看：

- snapshot / output / events 如何落盘
- waiting 状态如何映射为 `human_input_required`
- 单 run 存储语义当前锁定了什么

### `internal/formula/runtime/executor_test.go`

重点看：

- runtime 执行主循环
- repair/fixer 行为
- workspace/environment 相关夹具
- 对 step 执行边界的底层约束

## 对 `tt-2tr` / `tt-4ce` 的直接建议

### 给 `tt-2tr`（最小验证场景矩阵）

优先从以下现有样例反推矩阵，而不是先抽象后找例子：

1. 单 run 内 loop 并发
2. schedule 触发重复 run
3. worktree 隔离执行
4. waiting human input → resume
5. dashboard reopen 单 run 状态

然后再标出“哪些地方一旦从单 run 变成多 run 就失效或需要聚合层”。

### 给 `tt-4ce`（MVP 边界）

建议把现有能力分成两类：

- **可直接复用的执行层拼图**
  - loop 并发
  - schedule 触发
  - worktree 隔离
  - resume/human input
  - nested formula / atomics
- **仍缺失的聚合层能力**
  - 多 run 分组 / supervisor 概念
  - 聚合状态与聚合事件流
  - 统一 dashboard / API
  - 跨 run human input ownership
  - 批次级 reopen / cancel / retry

这样可以避免 MVP 范围写得过大，也能避免把已有 runtime 拼图误记成“完全缺失”。
