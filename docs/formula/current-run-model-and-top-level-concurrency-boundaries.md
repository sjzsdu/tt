# Formula 当前运行模型与顶层并发边界（tt-iqj）

本文档梳理 `tt formula` 当前真实运行单位、生命周期与并发边界，目的是给多 formula 并发相关 beads 提供统一前提，避免把 loop 内并发误判为“顶层多个 formula 一起运行”。

## 1. 本 bead 在依赖树中的位置

- 本 bead：`tt-iqj`，关注 **当前单个 formula run 的真实执行模型**。
- 上游依赖：`tt-f08` 已关闭，已给出第一阶段产品方向——优先采用“多个独立 run + 统一监视视角”。
- 下游阻塞：
  - `tt-4ce`：MVP 方向与阶段边界；
  - `tt-d9e`：run store / dashboard 对统一监视的缺口；
  - `tt-q3o` / `tt-ueo`：workspace、副作用隔离与 step 安全矩阵。

因此本 bead 只回答：**现在系统里一个 formula run 到底是什么、哪些能力天然绑定单 run、哪里已有局部并发、哪些“看起来像多 run”其实不是。**

## 2. 结论先行

### 2.1 当前顶层执行模型是“单个 workflow run”

从 `cmd/formula/formula_run.go` 到 `internal/formula/runtime/executor.go` 的链路看，`tt formula run <name>` 的一次调用会：

1. 解析 formula 并编译出一个 `*ir.Workflow`；
2. 可选地创建一个 `run.Store`（对应一个 `run_id` 和一个 run 目录）；
3. 创建一个 typed runtime `Executor`；
4. 对 **这一个 workflow** 做一次 `Executor.Run(ctx)`；
5. 运行状态、事件、步骤产物、human input、repair、dashboard 全部围绕这个 **单一 run_id** 持久化和展示。

也就是说，当前顶层运行单位不是“任务批次”“run group”或“supervisor”，而是：

- **一次 CLI 调用**
- **一个 workflow**
- **一个 run_id**
- **一个 run store 目录**
- **一个 dashboard snapshot**

### 2.2 当前局部并发主要存在于 loop 内部，而不是顶层多个 formula

现有并发能力主要由 `loop` step 提供，关键语义在 `ai-docs/step-kinds-reference.md` 已明确：

- `for_each`
- `parallel = true`
- `max_concurrency`

这些能力表示：**在单个 workflow 内，对某个 loop body 的迭代做局部并发执行**。它们不会生成新的顶层 run，也不会创建新的 run store 目录，更不会让 resume / human input / dashboard 从“单 run 视角”升级成“多 run 聚合视角”。

因此：

- **loop 并发 = 单 run 内部的子任务并发**；
- **不是顶层多个 formula run 并发**。

### 2.3 现有大量能力是 run-scoped，这是多 formula 并发研究的根约束

以下能力都天然绑定单个 run：

- `run_id` 与 run 目录；
- `run.json` / `workflow.json` / `state.json` / `logs.jsonl`；
- `steps/<step-id>.*` 产物；
- resume；
- waiting input / human input response；
- stop / interrupt；
- repair 记录与确认；
- dashboard snapshot 与 final report chat。

这意味着若未来要支持“多个 formula 一起跑并统一监视”，第一阶段最顺的路径仍然是：

- **保持多个独立 run**；
- 在展示层或上层调度层做聚合；
- 而不是先改写 runtime 的顶层单 run 模型。

## 3. 单个 formula run 的启动 → 执行 → 持久化 → 恢复 → 展示链路

### 3.1 启动入口：`cmd/formula/formula_run.go`

`runFormulaRun` 的主流程可概括为：

1. `formula.NewParser(...).LoadByName(name)`：按名称加载 formula；
2. `formula.ResolveFormulaByName(...)`：解析变量、继承、展开后的 formula；
3. `formula.WorkflowFromFormula(resolvedFormula)`：转成 typed workflow；
4. `runFormulaPreflight(...)`：做 preflight；
5. `run.EnsureWorkspaceState(projectRoot)`：确保 `.tt/` 与 run 根目录可用；
6. `newFormulaPicoclawRuntime(projectRoot)`：构建 agent runtime；
7. `run.NewWithMetadata(...)`：创建 `run.Store`，得到唯一 `run_id`；
8. `newFormulaDashboardServer(workflow)`：构建与该 workflow 对应的 dashboard 状态；
9. `executeFormulaRecipeRuntime(...)`：装配 executor 并实际执行。

关键点：**顶层只创建一个 workflow 和一个 run.Store。**

### 3.2 执行主体：`internal/formula/runtime/executor.go`

`Executor.Run(ctx)` 的执行模型是：

1. `Store.StartWorkflow(workflowID)`：标记 workflow 开始；
2. `PlanTopological`：对 workflow graph 做拓扑排序；
3. `prepareWorkspace`：准备运行 workspace；
4. 依次遍历拓扑序中的 node；
5. 对每个 step：
   - 检查 context / condition；
   - 更新 step state 为 running；
   - 调用对应 typed step 的 `Run`；
   - 写入 output / error / wait / repair；
   - 更新状态并发出 event；
6. 全部完成后 `FinishWorkflow(completed)`。

从这里能看到：

- runtime 的单位是 **一个 workflow graph**；
- `RunResult` 也是一个 workflow 级结果；
- event sink 默认发的是 `workflow.*` / `step.*` 事件；
- 没有“多 workflow 顶层调度器”这一层。

### 3.3 持久化桥接：`internal/formula/runtime/formularun_store.go`

`FormulaRunStateStore` 把 typed runtime 的内存状态镜像到旧的 run artifact 布局：

- runtime `Snapshot` → `state.json`
- runtime `Event` → `logs.jsonl`
- step output / error / await → `steps/<step-id>.*`
- `RepairRecord` → `patches/<run-id>.json`
- workflow status → `run.json`

这层是关键约束：它证明现有 runtime 虽然是 typed executor，但对外暴露和被 dashboard / resume 消费的，仍然是 **一个 run 目录**。

### 3.4 展示与恢复：`run.Store` + dashboard + resume/input

`internal/formula/run/store.go` 定义了单 run 的持久化模型：

```text
.tt/runs/formula/<formula-slug>/<run-id>/
  run.json
  workflow.json
  state.json
  logs.jsonl
  steps/*
  patches/<run-id>.json
```

这套结构隐含了几个现实：

- `run.Resolve(...)`、`run.List(...)`、`run.Delete(...)` 都是围绕 **单个 run record**；
- `formula run open/show/rm/resume/input` 的 CLI 入口也都是基于 **选定某个 run_id**；
- dashboard 读取的是该 run 的 snapshot，而不是 run 集合。

## 4. resume / waiting input / stop / repair 对顶层并发边界的约束

### 4.1 resume 是 run-scoped，而不是 workflow 内任意子单元 scoped

`cmd/formula/formula_run_resume.go` 的 `runFormulaRunResume` 流程是：

1. `run.Resolve("", id)` 找到一个 run record；
2. 重新 compile 该 run 对应的 formula；
3. 读取该 run 的 snapshot；
4. 基于 snapshot 计算 `initialResults` / `initialContext`；
5. 重建 dashboard；
6. 调 `executeFormulaResume(...)` 继续执行同一个 run。

这说明 resume 的语义不是“恢复某个 loop iteration”或“恢复某个子 workflow”，而是：

- **恢复这一个 run_id 对应的 workflow 执行**。

另外，`resumeDependencyExclusions(...)` 明确对 loop 祖先做特殊处理，说明 loop 并发仍然只是单 run 内部结构，不是独立顶层 run。

### 4.2 waiting input / human input 也是 run-scoped

`runFormulaRunInput` 的逻辑是：

1. 先解析一个 run record；
2. 要求该 run 的 metadata 状态是 `waiting_input`；
3. 找到该 run snapshot 中等待输入的 step；
4. 把 response 写回该 run 目录中的 step artifacts；
5. 重置该 run metadata，然后直接触发 resume。

约束点在于：

- waiting 状态写在单个 `run.json`；
- human input request / response 文件挂在单个 run 下；
- input 提交之后恢复的是同一个 run。

如果未来一个聚合视图中同时存在多个 waiting runs，当前模型并没有“批量 human input”或“上层 supervisor 级 input broker”。

### 4.3 stop / interrupt 目前也以单 run 为边界

`runFormulaRun` 和 `executeFormulaResumeRuntime` 都通过 `signal.NotifyContext(..., os.Interrupt)` 绑定当前执行上下文。

这意味着：

- Ctrl-C 中断的是当前这一个 CLI 进程里的执行；
- 状态回写的是这一个 run 的 interrupted / failed 路径；
- 当前没有“一个父控制器同时优雅停止多个独立 runs”的现成抽象。

因此 stop request 在现有系统中更接近：

- **单执行上下文 / 单 run 的终止能力**。

### 4.4 repair 是 run-scoped 的历史与交互能力

`Executor.tryFixAndRerun(...)` 和 `recordRepair(...)` 表明：

- repair 发生在某一个 step 的失败/校验失败之后；
- 记录写入该 workflow 对应的 runtime snapshot；
- 再由 `FormulaRunStateStore.SaveRepair(...)` 落盘到该 run 目录下的 `patches/<run-id>.json`；
- dashboard 的 `Repairs` 面板展示的也是该 run 的 repair 列表。

所以 repair 不是全局事件流，也不是批次级事件流，而是：

- **单 run 下的 step 修复历史**。

这对多 formula 统一监视的含义是：聚合视图最多先做 repair 摘要或跳转入口；若直接做批量 repair 操作，会立即跨越当前 bead 范围并触碰下游 `tt-d9e`。

## 5. 哪些能力最接近“多运行”，哪些只是表面相似

### 5.1 最接近多运行的现有能力

#### A. `tt formula runs` / `run open`

它们已经支持：

- 列表化多个历史 runs；
- 选择一个 run 查看详情或打开 dashboard。

但这更像：

- **多条独立 run 记录的浏览能力**，
- 不是同时执行多个 run 的控制平面。

#### B. dashboard 的单 run 观察能力

dashboard 已有：

- step 状态；
- human input；
- repairs；
- final report chat；
- graceful stop 提示。

这些很适合作为未来统一监视视角里的“单 run drill-down 页面”，但本身仍是 **一个 snapshot 对一个 run**。

#### C. loop 的并发能力

loop 是当前最容易被误判成“已经支持并发”的地方。它确实支持并发，但仅限：

- 单 workflow 内；
- 单 executor 内；
- 单 run store 内；
- 单 dashboard snapshot 内。

因此它只是**局部并发语义**，不是多 formula 顶层并发语义。

### 5.2 只是表面相似、不能等同于多 formula 顶层并发的能力

#### A. `loop.parallel` / `max_concurrency`

它们解决的是：

- 一个 run 内部怎样并发处理一批迭代。

它们没有解决：

- 多个 workflow 的 run id 管理；
- 多 run 的独立恢复；
- 多 run 的统一停止；
- 多 run 的聚合展示与交互。

#### B. `retry`

`retry` step 是单 run 内部的局部重试控制结构，不是多 run supervisor。

#### C. repair

repair 看起来像“失败后再跑一次”，但它仍发生在：

- 某一步；
- 某个 run；
- 当前 executor 的上下文内。

它不是“重启另一个 formula run”。

#### D. final report chat

它是 run-scoped session（类似 `<run-id>:final-report-chat`），说明交互会话本身也绑定单 run，而不是多 run 聚合对话。

## 6. 对后续多 formula 并发研究的直接约束

基于当前实现，后续 beads 需要默认接受以下边界：

1. **顶层运行单位当前就是单 run。**
   - 不应把 loop 并发当成“系统已具备顶层多 formula 并发”。

2. **run store 是单 run 持久化模型。**
   - 后续若做统一监视，优先考虑在其上做聚合读取，而不是先破坏现有目录与 metadata 结构。

3. **resume / waiting input / repair / stop 都绑定单 run。**
   - 多 run 能力如果要做第一阶段 MVP，优先提供摘要、跳转和单 run drill-down；
   - 不要默认把批量交互纳入 phase 1。

4. **当前最接近目标的路径是“多个独立 runs + 聚合观察”，不是“把一个 workflow 变成多 workflow 容器”。**
   - 这与上游 bead `tt-f08` 的结论一致。

## 7. 给下游 beads 的可复用结论

### 对 `tt-4ce`（MVP 边界）

可直接继承：

- 顶层单 run 是现状；
- MVP 更适合“多独立 run + 聚合监视”；
- phase 1 不应把批量 resume / 批量 human input / 批量 repair 作为默认范围。

### 对 `tt-d9e`（run store / dashboard 缺口）

可直接继承：

- 现有 store、snapshot、dashboard 都是单 run shaped；
- 统一监视若要成立，需要聚合多个单 run 记录，而不是复用 loop 内状态。

### 对 `tt-q3o` / `tt-ueo`（workspace / 副作用 / 安全矩阵）

可直接继承：

- 现有顶层并发并不存在；
- loop 内并发与多 formula 顶层并发在 workspace、副作用、幂等性、安全性上不能混为一谈。

## 8. 参考模块

- `cmd/formula/formula_run.go`
- `cmd/formula/formula_run_resume.go`
- `internal/formula/runtime/executor.go`
- `internal/formula/runtime/formularun_store.go`
- `internal/formula/run/store.go`
- `ai-docs/step-kinds-reference.md`
- `ai-docs/architecture.md`
- `ai-docs/module-map.md`
