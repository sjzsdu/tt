# 多 formula 运行能力的最小验证场景矩阵（tt-2tr）

本文档定义多 formula 运行能力 phase 1 MVP 的最小验证场景矩阵，供后续 implementation beads、手工验收和自动化测试转写直接复用。本文档不实现功能，只基于已完成的 research / decision beads 收敛统一的验证骨架。

## 1. 本 bead 在依赖树中的位置

- 当前 bead：`tt-2tr`，目标是在 **不实现多 run 功能** 的前提下，先定义未来验证该方向是否成立的最小场景矩阵。
- 已完成上游依赖：
  - `tt-0gu`：盘点了 loop 并发、schedule、run reopen、worktree、resume/human input 等可复用样例与测试入口；
  - `tt-d9e`：确认 run store / dashboard / web API / 前端状态天然是 **single-run shaped**，聚合监视必须新增聚合层；
  - `tt-q3o`：确认共享 cwd 默认不安全，worktree 只能解决部分隔离问题；
  - `tt-ueo`：给出 step 类型安全矩阵，明确 waiting input / repair / final report chat 不应默认被抬到聚合层；
  - `tt-4ce`：已把 phase 1 MVP 收敛为 **多个独立 run 的批量发起 + 聚合监视 + 单 run drill-down**。
- 本 bead 无下游依赖树展示，但它会直接约束后续 implementation / validation beads 的验收口径。

因此本文档只回答：

1. phase 1 MVP 至少必须验证哪些场景；
2. 每个场景要观察什么、依赖什么现有夹具或交互路径；
3. 哪些属于 MVP 必测，哪些属于后续扩展；
4. 如何把这些场景直接转写为测试或验收用例。

## 2. 验证范围与总原则

### 2.1 验证对象

本文默认验证对象是 `tt-4ce` 已确认的 phase 1 方向：

> **多个独立 formula run 的批量发起 + 聚合监视 + 单 run drill-down**

因此矩阵里的所有场景都基于以下共同前提：

- 顶层仍是多个 **独立 run**，不是父子 formula 编排；
- 单 run 的 runtime、resume、human input、repair、final report chat 语义保持不变；
- 聚合页优先承担 **摘要、告警、跳转**，不承担完整批量交互控制台；
- 高风险 step / 共享 workspace / 外部副作用场景首先作为边界与限制来验证，而不是当作默认支持能力。

### 2.2 验证总原则

后续无论是手工验收还是自动化测试，都应遵循：

1. **先证明确实是多独立 run，不是单 run loop 并发伪装。**
2. **先验证聚合可见性，再验证交互跳转。**
3. **先验证低风险场景成立，再验证高风险场景被正确限制或明确告警。**
4. **waiting input / repair / final report chat 在 MVP 中默认以单 run drill-down 保真为目标，而不是批量操作能力。**

## 3. 与现有测试夹具 / 代码模块的映射

本文档并不新增自动化测试实现，但后续实现 bead 应优先复用以下现有入口：

| 目标 | 现有模块 / 夹具 | 可复用点 |
| --- | --- | --- |
| 单 run runtime / snapshot / dashboard 映射 | `cmd/formula/formula_runtime_test.go` | event sink、snapshot 映射、workspace guard、loop session 隔离 |
| waiting input / human input CLI 路径 | `cmd/formula/formula_human_input_test.go` | waiting step 解析、表单字段与提交校验 |
| run store / 状态落盘 | `internal/formula/runtime/formularun_store_test.go` | completed / waiting / repairs 如何镜像到 run artifacts |
| 顶层运行执行器 | `internal/formula/runtime/executor_test.go` | workflow 级执行、workspace/environment、step 执行边界 |
| dashboard reopen / state 更新 | `cmd/formula/formula_dashboard_updates_test.go`、`cmd/formula/formula_run_records.go` | 单 run reopen 详情页仍需保持可用 |
| 单 run 详情前端 | `web/apps/formula/src/components/App.tsx` 及其 hook / api / types | 当前 snapshot-based 单 run 详情模型，应作为 drill-down 保留 |

这些映射意味着：phase 1 的新增测试很可能不是替换这些夹具，而是在其之上新增“聚合页 + 多 run 摘要读模型 + 跳转桥接”的测试层。

## 4. MVP 最小验证场景矩阵

以下场景按照“**MVP 必测**”与“**后续扩展**”分层。每个场景都包含：验证目标、前提条件、建议入口/夹具、预期观察结果。

### 4.1 场景 A：只读并发 run 的聚合监视（MVP 必测）

**类别**：只读并发 / 聚合监视  
**优先级**：MVP 必测

**验证目标**

验证系统能同时承载多个彼此独立、低风险、只读型 formula runs，并在聚合视图中正确展示各 run 的基础摘要，而不要求引入父子编排语义。

**前提条件**

- 选择两个或以上只读型 formula / workflow 样例；
- 不写共享目录，不触发 human input，不依赖 repair；
- 每个 run 都有独立 `run_id` 与单 run snapshot。

**建议入口 / 夹具映射**

- 参考 `cmd/formula/formula_runtime_test.go` 中单 run workflow 执行与 dashboard snapshot 映射；
- 复用 `internal/formula/runtime/formularun_store_test.go` 里的 run artifact 落盘行为；
- 聚合层新增后，应在其上读取多条 `run.Metadata` / state 摘要。

**预期观察结果**

- 聚合页或聚合 API 能列出所有 run；
- 每条摘要至少包含：`run_id`、formula 名称、状态、开始时间、完成时间或当前活跃状态；
- 运行中的 run 与已完成 run 能被区分；
- 点进任意 run 后，原有单 run 详情仍可正常展示 step / log / output。

**失败信号**

- 聚合层只看见最新一个 run；
- 多个 run 被错误合并成一个 snapshot；
- drill-down 后详情页丢失或显示错 run。

### 4.2 场景 B：worktree 隔离下的 repo 写操作并发（MVP 必测）

**类别**：worktree 隔离并发  
**优先级**：MVP 必测

**验证目标**

验证对于需要修改仓库工作树的 formula，phase 1 只在独立 workspace / worktree 前提下承诺并发成立，并且聚合视图能暴露隔离信息而不是掩盖风险。

**前提条件**

- 至少两个 run 会在 repo 内产生写操作；
- 每个 run 启用独立 worktree workspace；
- 不要求远端 push；
- 输出路径不共享或明确按 run 隔离。

**建议入口 / 夹具映射**

- 参考 `internal/formula/runtime/workspace.go` 与 `executor_test.go`；
- 参考 `cmd/formula/formula_dashboard_paths_test.go` 和 `formula_runtime_test.go` 中 workspace guard 相关行为；
- 聚合层应读取并展示 `workspace_dir` 或等价隔离摘要。

**预期观察结果**

- 两个 run 都能启动并完成或进入各自独立状态；
- 聚合摘要能显示每个 run 的 workspace / 隔离线索；
- 单 run 详情页中展示的 workspace 路径互不相同；
- 不出现共享工作树文件相互覆盖的现象。

**失败信号**

- run 实际落在同一 workspace；
- 聚合层无法区分隔离模式；
- 一个 run 的 repo 变更污染另一个 run 的执行结果。

### 4.3 场景 C：共享 cwd / 共享输出路径冲突被正确限制（MVP 必测）

**类别**：workspace 边界 / 负面样例  
**优先级**：MVP 必测

**验证目标**

验证 phase 1 不把“默认共享 cwd 并发”误包装为安全能力；对共享输出目录、共享工作树写操作等高风险场景，要么显式拒绝、要么强告警、要么在验收文档中明确列为不支持。

**前提条件**

- 选择会写共享路径的 formula 样例；
- 不启用 worktree，或显式把多个 run 指向同一输出目录；
- 至少一个步骤具有文件覆盖风险（例如 `write_files` 或写型 `script`）。

**建议入口 / 夹具映射**

- 参考 `docs/formula/workspace-and-side-effect-isolation-boundaries-for-multi-run.md`；
- 参考 `docs/formula/step-safety-matrix-for-top-level-multi-run.md` 中 `write_files` / 写型 `script` 的风险分层；
- 如后续实现加入 guardrail，应单独为该 guardrail 添加测试。

**预期观察结果**

- 产品层对该场景给出明确限制；
- 若实现支持预检，应返回可解释的错误 / 警告；
- 若暂不做自动预检，验收文档中必须明确该场景不属于 MVP 支持范围。

**失败信号**

- 系统默认为安全并发并产生静默文件覆盖；
- 聚合监视页看起来正常，但底层共享路径已互相污染；
- 文档、实现、验收口径三者不一致。

### 4.4 场景 D：聚合监视中的状态分布与 attention 定位（MVP 必测）

**类别**：聚合监视 / attention 线索  
**优先级**：MVP 必测

**验证目标**

验证聚合层不只是“列出 runs”，还应帮助用户快速定位：哪些 run 在运行、哪些已完成、哪些失败、哪些等待人工处理。

**前提条件**

- 至少构造 running / completed / failed / waiting_input 中的三类状态组合；
- 不要求批量操作能力；
- 单 run 详情页已能表达这些状态。

**建议入口 / 夹具映射**

- `formularun_store_test.go` 已覆盖 completed / waiting / repairs 的落盘语义；
- `formula_runtime_test.go` 已覆盖 workflow completed、repair recorded 等 snapshot 映射；
- 聚合层新增后，应有 group summary / run summary 级测试。

**预期观察结果**

- 聚合页或 API 至少能给出状态分布计数；
- waiting input / failed run 能在列表层被看见；
- 用户无需逐个点开所有 run 就能识别 attention items；
- 聚合页可跳转到对应 run 的详情页继续处理。

**失败信号**

- 聚合层只显示 total 数量，没有 attention 维度；
- waiting / failed run 被淹没在普通完成项中；
- attention 项无法定位到具体 run。

### 4.5 场景 E：waiting input run 与普通 run 并存时的单 run drill-down（MVP 必测）

**类别**：waiting input / 交互边界  
**优先级**：MVP 必测

**验证目标**

验证当多个 run 并存，且其中某个 run 进入 `waiting_input` 时，聚合层至少能正确暴露该 attention 状态，并把用户带到正确的单 run 详情或单 run 输入路径；同时不要求聚合层直接承载批量 human input。

**前提条件**

- 至少一个 run 进入 waiting_input；
- 同时存在其他 running 或 completed run；
- 单 run input / resume 链路保持现有语义。

**建议入口 / 夹具映射**

- 直接复用 `cmd/formula/formula_human_input_test.go` 的 waiting step 解析与字段校验；
- 参考 `current-run-model-and-top-level-concurrency-boundaries.md` 中对 run-scoped input / resume 的约束；
- 聚合层只需验证“看见 + 跳转 + 正确归属”。

**预期观察结果**

- 聚合页将该 run 标记为 waiting input；
- drill-down 或等价操作能定位到正确的 run / step；
- 提交输入后恢复的是原 run，而不是错误 run；
- 其他 runs 的状态不被错误改变。

**失败信号**

- 聚合页无法区分哪个 run 在等待；
- 用户可能把输入提交给错误 run；
- input 提交后触发了错误的 run resume。

### 4.6 场景 F：取消 / 停止请求的边界可见性（MVP 必测）

**类别**：取消 / 恢复边界  
**优先级**：MVP 必测

**验证目标**

验证在多 run 聚合视角下，stop / interrupt 仍然保持 run-scoped 语义；至少要让用户看见哪个 run 被请求停止，而不是误以为 group 级 stop 已实现。

**前提条件**

- 至少一个 run 在执行中；
- 另一个 run 处于已完成、失败或等待状态之一；
- 当前实现若仅支持单 run stop，则聚合层只验证摘要与归属。

**建议入口 / 夹具映射**

- 参考 `cmd/formula/formula_dashboard_handlers.go` 中 `POST /api/stop` 的单 run handler；
- 参考 `current-run-model-and-top-level-concurrency-boundaries.md` 对 stop/interrupt 为单执行上下文能力的结论；
- 若后续实现提供聚合页跳转/提示，应为此新增测试。

**预期观察结果**

- 聚合层能显示某个 run 已请求停止或已中断；
- 用户能定位该 stop 影响的是哪个 run；
- 不会把单 run stop 描述成整个 group 的统一取消。

**失败信号**

- 聚合页对 stop 完全不可见；
- 展示语义暗示“整批都已取消”，但实际只影响某一个 run；
- run 归属不明。

### 4.7 场景 G：run reopen / 历史聚合可见性（MVP 扩展，非首批必测）

**类别**：恢复浏览 / 历史批次  
**优先级**：后续扩展

**验证目标**

验证已结束的一组 runs 是否仍能被重新聚合浏览，以及从聚合页重新打开单 run dashboard 的能力是否保留。

**前提条件**

- 多个 run 已落盘；
- 至少包含 completed / failed 的混合历史；
- 单 run reopen 已可用。

**建议入口 / 夹具映射**

- 参考 `cmd/formula/formula_run_records.go` 与 dashboard reopen 相关测试；
- 复用 `run.List()` 作为历史记录枚举基线。

**预期观察结果**

- 可以从历史 group 或等价聚合入口重新看到摘要；
- 可以打开任一旧 run 详情页；
- 不要求首批实现 live group websocket replay。

### 4.8 场景 H：repair / retry-step 仅保留单 run 详情能力（MVP 扩展，边界验证）

**类别**：repair / 恢复交互边界  
**优先级**：后续扩展

**验证目标**

验证聚合层不会在 MVP 过早承诺批量 repair / retry-step，但至少能标出某个 run 存在 repair attention，并允许用户跳到单 run 详情处理。

**前提条件**

- 至少一个 run 生成 repair 记录；
- repair 仍按单 run artifacts 落盘；
- 另一个 run 处于非 repair 状态，便于验证归属清晰。

**建议入口 / 夹具映射**

- `formula_runtime_test.go` 中 `TestFormulaRuntimeDashboardEventSinkRecordsRepairs`；
- `formularun_store_test.go` 中 repair artifact 落盘；
- `step-safety-matrix-for-top-level-multi-run.md` 对 repair 的 run-scoped 定位。

**预期观察结果**

- 聚合摘要能看出某 run 有 repair attention；
- 具体确认 / retry 在单 run 详情内完成；
- 产品文档不把批量 repair 写成 MVP 既有能力。

### 4.9 场景 I：final report chat 仍是单 run 后处理能力（后续扩展）

**类别**：final report / 单 run 会话边界  
**优先级**：后续扩展

**验证目标**

验证聚合层即使展示某个 run 已完成且有 final output，也不要求在 MVP 中统一承载多个 run 的 final report chat；chat 仍应以单 run 详情为准。

**前提条件**

- 至少一个 run 有 final output；
- 单 run final report chat 仍保持现有路径与 session 语义；
- 聚合层只暴露完成态与 drill-down 入口。

**建议入口 / 夹具映射**

- 参考 `formula_runtime_test.go` 中 final report chat handler 相关测试；
- 参考前端 `App.tsx` 中 `FinalReportModal` 依赖单 snapshot 的现状。

**预期观察结果**

- 聚合层可定位“某 run 有可查看 final output”；
- 进入详情后 final report chat 正常工作；
- 不要求聚合页直接发送 chat message。

## 5. 推荐的 MVP 验收最小集合

若后续实现需要一个最小、统一、可执行的 phase 1 验收骨架，建议至少覆盖以下 **6 个 MVP 必测场景**：

1. **场景 A**：只读并发 run 的聚合监视；
2. **场景 B**：worktree 隔离下的 repo 写操作并发；
3. **场景 C**：共享 cwd / 共享输出路径冲突被正确限制；
4. **场景 D**：聚合监视中的状态分布与 attention 定位；
5. **场景 E**：waiting input run 与普通 run 并存时的单 run drill-down；
6. **场景 F**：取消 / 停止请求的边界可见性。

这 6 个场景共同覆盖了 bead 描述要求的五大类别：

- 只读并发
- worktree 隔离并发
- 聚合监视
- 取消 / 恢复边界
- waiting input / 交互边界

并且与 `tt-4ce` 收敛出的 phase 1 范围一致：

- 做聚合可见性与单 run drill-down；
- 不提前承诺批量 repair / 批量 human input / 聚合 chat 控制台；
- 把共享 workspace / 副作用风险当作必须被验证的限制条件。

## 6. 建议如何转写为后续测试 / 验收用例

### 6.1 测试分层建议

后续 implementation beads 在落地测试时，可按以下层次拆分：

1. **读模型 / store 层测试**
   - 多 run 摘要枚举；
   - group / batch 归组信息读取；
   - 状态分布计算；
   - waiting / repair / stop attention 归类。

2. **服务端 API / handler 测试**
   - 聚合状态接口；
   - 聚合列表接口；
   - 从聚合项跳转到单 run 详情所需的定位字段；
   - 不支持的聚合交互是否正确报错或不暴露。

3. **前端组件 / 集成测试**
   - 聚合列表页状态呈现；
   - waiting / failed / completed attention 标记；
   - drill-down 到单 run 页；
   - 单 run 详情现有行为不回归。

4. **手工验收剧本**
   - 同时启动多个样例 formula；
   - 观察聚合页；
   - 对 waiting / failed run 做 drill-down；
   - 验证共享 cwd 场景被限制。

### 6.2 用例命名建议

为了与现有测试文件风格保持一致，可使用类似命名：

- `TestFormulaRunGroupSummaryListsIndependentRuns`
- `TestFormulaRunGroupSummaryShowsWorkspaceIsolation`
- `TestFormulaRunGroupRejectsSharedMutableWorkspaceScenario`
- `TestFormulaRunGroupSummaryHighlightsWaitingInputRuns`
- `TestFormulaRunGroupSummaryPreservesRunScopedStopState`
- `TestFormulaRunGroupDrillDownKeepsSingleRunHumanInputFlow`

这些名字不是本 bead 的实现要求，只是为了让后续自动化测试更容易直接从矩阵映射回验收目标。

## 7. 明确不在本 bead 解决的事项

本文档明确不解决：

- 多 run 功能实现；
- 聚合对象最终命名（group / batch / supervisor）；
- 最终 API schema；
- 最终前端路由设计；
- 完整 CI / 全矩阵自动化测试；
- 批量 human input / 批量 repair / 批量 final report chat 等高级交互。

这些内容都应由后续 implementation beads 在本文定义的验收骨架上继续细化。

## 8. 最终摘要

phase 1 的最小验证矩阵应围绕以下核心问题展开：

> **多个独立 run 是否能被安全地一起看见、被正确地区分、被准确地定位，并在高风险场景下保持明确边界。**

因此，MVP 的最小验收口径不是“任意多 formula 都能并发成功”，而是：

- 低风险独立 run 能聚合可见；
- workspace / 输出路径风险被显式限制；
- waiting / stop / failed 等 attention 状态能在聚合层被看见；
- 所有复杂交互仍可通过单 run drill-down 保真处理。

这组场景足以为后续 implementation beads 提供统一、可追踪、可转写为测试的验收骨架。