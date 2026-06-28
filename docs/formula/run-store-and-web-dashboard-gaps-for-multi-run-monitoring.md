# Run store 与 Web dashboard 对多 formula 统一监视的缺口（tt-d9e）

本文档梳理当前 `tt formula` 的 run metadata、snapshot、logs、dashboard API 与前端视图为什么天然以 **单 run** 为中心，并识别若要扩展为“在 web 中统一监视多个 formula”，最可能需要补充的概念边界与聚合层。

## 1. 本 bead 在依赖树中的位置

- 本 bead：`tt-d9e`，关注 **run store / dashboard / web API 为什么天然是单 run shaped**，以及统一监视需要新增哪些上层概念。
- 上游依赖：
  - `tt-f08` 已关闭：已将第一阶段问题定义收敛为“多个独立 run + 统一监视视角”；
  - `tt-iqj` 已关闭：已确认当前真实顶层执行单位是 **单个 workflow run**，大量能力都是 run-scoped。
- 下游阻塞：
  - `tt-2tr`：多 formula 运行能力的最小验证场景矩阵；
  - `tt-4ce`：多 formula 运行能力 MVP 方向与阶段边界。

因此本 bead **不实现** 聚合 dashboard，也**不设计最终 schema**；只回答：

1. 当前 CLI、run store、dashboard、web API 各自如何表现出“单 run 形状”；
2. 若未来要统一监视多个 run，最少缺哪些上层概念；
3. 哪些现有页面/API 可以扩展，哪些天然需要新增聚合层；
4. 一个“聚合视角”最小可用数据清单应包含什么。

## 2. 结论先行

### 2.1 当前所有核心读写面都以单 run 为主键

从 `internal/formula/run/store.go`、`internal/formula/ui/state.go`、`cmd/formula/formula_dashboard*.go`、`web/apps/formula/src/api/index.ts` 可以看到：

- run store 的主实体是 **`run.Record` / `run.Metadata`**；
- dashboard 内存态与落盘态都是 **一个 `ui.Snapshot`**；
- `/api/state` 返回的是一个 `state` 消息，payload 只有 **一个 snapshot**；
- `/ws` 广播的也是 **一个 snapshot 的持续更新**；
- 前端 hook `useFormulaDashboard()` 维护的是 **单个 `snapshot` state**；
- stop / retry / human input / repair confirm / final report chat 等交互接口，都默认“当前页面只附着到一个 run”。

也就是说，现有 Web dashboard 并不是“缺少一个列表组件”这么简单，而是**从存储模型、服务端 API 到前端状态管理都天然是 single-run shaped**。

### 2.2 已有能力更像“单 run 详情页”，不是“多 run 监控台”

当前 dashboard 很适合承担未来统一监视系统中的 **drill-down 明细页**：

- 看一个 run 的步骤状态；
- 提交这个 run 的 human input；
- retry 这个 run 里失败的 step；
- 查看这个 run 的 repair 记录；
- 在这个 run 的 final report chat 上继续交互。

但它不直接适合承担“多个 run 的统一监控主页”，因为当前没有：

- run group / batch / supervisor 等聚合对象；
- 多 run 摘要 API；
- 多 run 事件流或聚合 websocket；
- 将 stop / waiting input / failed / completed 跨 run 汇总的读模型。

### 2.3 第一阶段更合理的是新增“聚合层”，而不是把现有单 run 详情结构硬改成多 run

结合 `tt-f08` 与 `tt-iqj`，更顺的方向是：

- 保持 **多个独立 run** 的 store、snapshot、resume、input、repair 语义不变；
- 在它们之上新增一个 **聚合读模型 / 聚合 API / 聚合页面**；
- 现有 dashboard 页面继续作为“选中某个 run 后的详情页”。

这意味着 phase 1 更像：

- 新增“多 run 监控视图”；
- 保留“单 run dashboard 详情视图”。

而不是：

- 直接把 `ui.Snapshot` 扩展为“内嵌多个 run 的巨型对象”。

## 3. 当前系统为什么天然是单 run 形状

### 3.1 CLI / run store：一个目录对应一个 run

`internal/formula/run/store.go` 暴露的核心事实：

- `Metadata` 直接以 `RunID` 为主键；
- `Store` 只绑定一个 `Dir` 和一个 `Meta`；
- `NewWithMetadata(...)` 一次只创建一个 run 目录；
- `Finish(...)`、`MarkWaitingInput(...)`、`AppendEvent(...)`、`SaveState(...)` 都只写当前这一个 run；
- `Resolve(root, id)` 返回一个 `Record`；
- `List(root)` 返回的是 **多个独立 `Record` 列表**，但每个 record 仍然是单 run。

目录布局也是单 run 为单位：

```text
.tt/runs/formula/<formula-slug>/<run-id>/
  run.json
  workflow.json
  state.json
  logs.jsonl
  steps/*
  patches/<run-id>.json
```

这说明当前 store 层能做的是：

- 枚举多个历史 run；
- 解析一个 run；
- 删除一个 run；

但还没有：

- “一批 run 属于同一次顶层启动”的 group metadata；
- 父 run / 子 run 关系；
- 聚合状态文件；
- 跨 run 的统一事件索引。

### 3.2 snapshot 模型：整个 dashboard 状态只有一个 run 的上下文

`internal/formula/ui/state.go` 中的 `Snapshot` 字段本身就体现了这一点：

- `RecipeName`
- `Description`
- `Phase`
- `Status`
- `FinalOutput`
- `FinalReportChat`
- `Repairs`
- `Vars`
- `Steps`
- `Edges`
- `Logs`
- `WorkspaceDir`
- `RunID`
- `StopRequested`

这里没有任何数组或 map 用于表示“多个 runs 的集合”。

尤其关键的是，下列字段全部默认只描述一个 run：

- `RunID`：当前快照的唯一 run；
- `Steps`：当前 workflow 的步骤列表；
- `Edges`：当前 workflow graph；
- `FinalReportChat`：当前 run 的对话会话；
- `Repairs`：当前 run 的修复记录；
- `Logs`：当前 run 的日志流。

因此 `ui.Snapshot` 的职责天然更像“**单 run dashboard 详情页状态**”。

### 3.3 服务端 dashboard：一个 server 只 attach 一个 store

`cmd/formula/formula_dashboard.go` / `formula_dashboard_handlers.go` 暴露出更明确的单 run 假设：

- `formulaDashboardServer` 只有一个 `state ui.Snapshot`；
- 只有一个 `store *run.Store`；
- `attachStore(store)` 只附着一个 run store，并将 `state.RunID = store.Meta.RunID`；
- `handleState` 返回单个 snapshot；
- `broadcast()` 广播单个 snapshot message；
- `persistSnapshot()` 把当前状态写回当前 store 的 `state.json`。

服务端 API 也全部围绕“当前 dashboard 所附着的那个 run”：

- `POST /api/stop` → 给该 run 写 `stop-requested`；
- `POST /api/human-input` → 给该 run 的 waiting step 提交输入；
- `POST /api/retry-step` → 重试该 run 的失败 step；
- `POST /api/repairs/confirm` → 确认该 run 的 repair；
- `POST /api/final-report-chat*` → 操作该 run 的 final report chat；
- `GET /api/agent-session` → 基于该 run 的 workspace 找 transcript。

这些 handler 都没有 `run_id` 参数，因为设计前提是：**当前页面已经绑定到某一个 run**。

### 3.4 Web API 与前端 hook：只消费一个 snapshot

`web/apps/formula/src/api/index.ts` 与 `hooks/useFormulaDashboard.ts` 的契约也很单一：

- `api.state(): Promise<FormulaDashboardSnapshot>`；
- websocket message 类型是 `{ type: 'state', state: FormulaDashboardSnapshot }`；
- `useFormulaDashboard()` 内部 state 是 `snapshot: FormulaDashboardSnapshot | null`；
- 汇总 `summary` 时，只对这一个 snapshot 的 `steps` / `logs` / `repairs` 计数；
- `orderedSteps` 只排序当前 snapshot 的 steps。

前端类型 `FormulaDashboardSnapshot` 也与 Go 端 `ui.Snapshot` 一一对应，只服务于一个 run 的详情展示。

## 4. 统一监视多个 formula 至少缺哪些概念

如果要在 web 中统一监视多个 formula，至少缺以下上层概念。

### 4.1 聚合对象：run group / batch / supervisor

当前系统只有 run，没有“这批 runs 属于同一次顶层动作”的概念。

因此至少需要一个聚合对象来回答：

- 这几个 run 为什么会被放在一起观察？
- 它们是同一次用户触发、同一批 schedule，还是同一个上层 formula fan-out？
- 聚合页的标题、创建时间、状态、来源是什么？

命名不必在本 bead 决定，但概念上至少需要下列之一：

- `run_group`
- `batch_id`
- `parent_run`
- `supervisor_run`

如果没有这层对象，前端最多只能展示“最近 N 个 run 的列表”，却无法表达“它们属于同一次多 formula 运行”。

### 4.2 聚合状态：单个 run 状态之外的 group 状态

现有 run status 只有单 run 级别，例如：

- running
- completed
- failed
- waiting_input
- interrupted
- stale

但统一监视时需要额外回答：

- group 是否仍在运行？
- group 是否部分完成？
- group 是否因某个 run waiting input 而卡住？
- group 是否有人为 stop 请求？

因此聚合层至少需要一种 group-level status 视角，例如：

- `running`
- `completed`
- `failed`
- `partial`
- `waiting_attention`
- `stopped`

本 bead不定义最终枚举，只指出**现有单 run status 不足以直接代表多 run 统一监视状态**。

### 4.3 聚合事件流：多 run 的 websocket / polling 入口

当前 `/ws` 只广播一个 snapshot 更新；`/api/state` 只拉一个 snapshot。

若要统一监视，至少需要新增一种聚合读接口：

- 返回 run 列表摘要；
- 提供 group 级 summary；
- 支持按 run_id drill-down；
- 能持续推送多个 run 的状态变化。

否则前端只能自己轮询 `run.List()` 再逐个打开多个 `/api/state`，既不高效也不符合现有 dashboard 模型。

### 4.4 交互归属：stop / human input / retry / repair / chat 要明确落在哪个 run

统一监视不是只读问题，还会遇到交互归属问题：

- 某个 waiting input 表单属于哪个 run、哪个 step？
- retry 某个失败 step 时，命令应该发给哪个 run？
- repair confirm 是确认哪个 run 的第几次 attempt？
- final report chat 是哪个 run 的 chat，会不会在聚合页直接发消息？

因此聚合页最少需要把每个可交互项保留明确的：

- `run_id`
- `formula`
- `step_id`（如适用）
- 当前交互类型

否则所有操作都会变成歧义操作。

## 5. 哪些现有页面 / API 可以扩展，哪些天然需要新聚合层

### 5.1 可以扩展复用的部分

#### A. `run.List()` 可作为聚合页的底层枚举入口

`run.List(root)` 已经能列出多个独立 run record，因此它适合作为未来聚合层的**底层原料**：

- 枚举候选 runs；
- 读取 metadata；
- 做最近运行列表或按 formula 过滤；
- 作为聚合 summary 的基础数据源。

但它本身只提供“独立 runs 的列表”，不提供 group 语义。

#### B. 单 run dashboard 可作为 drill-down 详情页

现有 `/api/state`、`/ws`、`useFormulaDashboard()`、前端详情 UI，适合继续承担：

- 点击某个 run 后进入详情；
- 查看该 run 的步骤、日志、repair、human input、final report chat。

也就是说，它们更适合保留为 **Run Detail View**。

#### C. 现有交互 handler 可继续作为单 run 操作面

现有 handler 不必一开始就改造成 group-aware。更现实的做法是：

- 聚合页只负责展示和定位；
- 具体 stop / retry / input / repair / chat，先跳转到单 run 页处理；
- 或者未来聚合 API 只是把 `run_id` 路由到现有单 run handler。

### 5.2 天然需要新增聚合层的部分

#### A. 聚合首页 / 多 run summary API

当前没有任何 endpoint 返回：

- 一组 runs 的摘要；
- 某个 group 下各 run 的状态分布；
- waiting / failed / completed 的计数；
- 最新活动时间；
- 每个 run 的关键入口（详情链接、workspace、错误摘要）。

这需要**新增 API 和读模型**。

#### B. 聚合 websocket / 增量更新流

当前 websocket 只有单 snapshot message。统一监视要么：

- 新增 group websocket，推送多 run 状态变化；
- 要么新增聚合 polling endpoint。

无论哪种，都不是对现有 `/ws` 的小改动，而是新增一个聚合分发层。

#### C. 聚合前端状态模型

当前前端所有类型与 hook 都以 `FormulaDashboardSnapshot` 为核心。统一监视至少需要新增例如：

- `RunSummary`
- `RunGroupSummary`
- `RunAttentionItem`
- `RunListResponse`

而不是让 `FormulaDashboardSnapshot` 同时扮演列表页和详情页模型。

## 6. 聚合视角的最小数据清单（不涉及最终 schema）

为了让后续 beads 能在不预设最终实现的前提下推进，这里给出一个“统一监视视角”最小数据清单。

### 6.1 group / page 级摘要

至少需要：

- 聚合对象标识（例如 group id / batch id / synthetic key）
- 标题或来源描述
- 创建时间 / 最近更新时间
- 总 run 数
- 状态分布（running / waiting / failed / completed 等计数）
- 是否存在需要人工关注的 run

### 6.2 每个 run 的摘要

至少需要：

- `run_id`
- formula 名称
- run status
- started_at / finished_at
- workspace_dir
- 错误摘要（如有）
- step 统计（总数 / completed / running / failed / waiting）
- 是否 stop requested
- 是否存在 human input
- 是否存在 repairs
- 跳转到单 run 详情页的入口

### 6.3 attention / action 线索

为了支持“统一监视”，至少需要能在列表层看见：

- 哪些 run 正在 waiting input
- 哪些 run 失败了
- 哪些 run 刚完成但有 final output 可查看
- 哪些 run 有 repair 待确认

注意这里是“看见并定位”，不是必须在本阶段直接做批量操作。

## 7. 对后续 beads 的直接复用价值

### 对 `tt-2tr`

本 bead可直接提供：

- 统一监视不等于重做 runtime；
- 最小验证场景应覆盖“聚合列表 + 单 run drill-down”；
- waiting / failed / completed / repair 这些 attention 维度应出现在验证矩阵里。

### 对 `tt-4ce`

本 bead可直接提供：

- MVP 更适合“新增聚合层 + 复用单 run 详情页”；
- 不应一上来把 `ui.Snapshot` 扩成多 run 大对象；
- phase 1 可先聚焦只读统一监视与 drill-down，谨慎纳入跨 run 交互。

## 8. 参考模块

- `internal/formula/run/store.go`
- `internal/formula/ui/state.go`
- `cmd/formula/formula_dashboard.go`
- `cmd/formula/formula_dashboard_handlers.go`
- `cmd/formula/formula_dashboard_updates.go`
- `web/apps/formula/src/api/index.ts`
- `web/apps/formula/src/hooks/useFormulaDashboard.ts`
- `web/apps/formula/src/types/index.ts`
- `docs/formula/current-run-model-and-top-level-concurrency-boundaries.md`
