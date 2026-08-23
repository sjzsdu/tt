# coder 产品开发系统规划书

> 最后更新：2026-08-23
> 状态：规划中，作为后续 `tt coder` 实施的共同上下文。

## 1. 背景与目标

`coder` 不是一个单独的“写代码 agent”，而是一个由人引领、多 agent 协作的软件产品设计、开发、验证和上线系统。

用户的核心诉求是：

- 人负责产品用途、方向、关键取舍和审核。
- Agents 负责提出产品功能、产品设计、架构方案、代码实现、测试验证、bug 查找和上线准备。
- 每个 agent 运行时都必须知道共同上下文，不能退化成几个孤立 agent 的松散组合。
- 产品功能、架构、设计稿、上线等关键点必须有人审。
- 人审适合使用动态表单承接反馈。
- 任务、决策、上下文、产物和审核结果必须持久化、可追溯。

一句话定位：

> `coder` 是建立在现有 `team`、`agent`、`formula`、dashboard 和动态表单能力之上的“人类产品负责人 + AI 软件团队”工作台。

## 2. 现有能力复用

优先复用现有系统，避免重新造 runtime。

### 2.1 `tt team`

已提供：

- 多 agent 团队运行时。
- 持久化 thread。
- 隔离 agent session。
- public discussion events。
- durable memory。
- shared blackboard。
- resume / ask / show / open。
- web dashboard。
- verification gate。
- 内置 `software-development` team。

公开文档已经描述 `software-development` team 可以把宽泛工程请求转成 repository-aware delivery：需求和架构探索、单写者实现、独立 review、测试验证、证据化最终结果。

### 2.2 `tt agent`

已提供：

- embedded agent registry。
- `coder` agent。
- `planner`、`tester`、`code-context` 等可组合角色。
- Picoclaw runtime 复用。
- agent web UI。
- session / transcript / git context 观察路径。

其中 `internal/agents/embedded/core/coder.md` 已将 `coder` 定义为：帮助用户完成编码、调研、实现、重构、测试和缺陷预防的默认工程 agent，并强调先理解上下文、定义验收标准、使用 `code-context`、持续验证。

### 2.3 `tt formula` 动态表单

`formula` 已有动态表单和 human input 能力：

- `execution = "human_input"` 支持固定表单。
- `form = true` 支持 agent 生成动态澄清表单。
- executor 会向 agent 注入 `tt-human-input` 协议。
- runtime 可返回 waiting 状态。
- dashboard 有 `/api/human-input` 提交入口。
- human input response 会保存为 step output，并触发 resume。

`coder` 不应重做动态表单引擎，而应复用或抽象这些能力，形成 coder 专用的 review gate。

## 3. coder 新增职责边界

现有 `team` 负责“agent 如何协作”。

`coder` 新增负责：

- 产品级上下文管理。
- 人审 gate 管理。
- 动态表单生成、展示、提交和审计。
- 任务图和里程碑管理。
- 决策、产物、测试证据、上线记录追溯。
- 面向用户的产品开发 dashboard。

不建议 `coder` 重新实现：

- LLM agent runtime。
- 多 agent 调度。
- 通用 dynamic form 协议。
- 基础 thread / memory / blackboard / verification。

## 4. 核心概念模型

### 4.1 CoderProject

代表一个软件产品项目。

```yaml
CoderProject:
  id: string
  name: string
  vision: string
  owner_intent: string
  target_users: []string
  status: exploring | planning | designing | developing | testing | releasing | live | paused
  current_thread_id: string
  created_at: timestamp
  updated_at: timestamp
```

### 4.2 CoderContextPacket

每个 agent 运行前都应拿到的产品级上下文切片。

```yaml
CoderContextPacket:
  id: string
  project_id: string
  thread_id: string
  version: int
  product:
    vision: string
    target_users: []string
    core_problem: string
    current_stage: string
    human_direction: string
    non_goals: []string
  phase:
    name: string
    objective: string
    success_criteria: []string
  decisions:
    accepted: []DecisionRef
    pending: []DecisionRef
  constraints:
    tech_stack: map
    deployment: map
    quality_bar: map
    budget_or_simplicity: string
  open_questions: []OpenQuestion
  agent_states: []AgentState
  task_graph: TaskGraphSummary
  artifacts: []ArtifactRef
  review_gates:
    current: ReviewGateRef
    completed: []ReviewGateRef
```

关键原则：每个 agent 不是拿到孤立任务，而是拿到“产品目标 + 当前阶段 + 自身职责 + 他人状态 + 历史决策 + 约束条件 + 当前人审状态”的完整工作切片。

### 4.3 ReviewGate

人的关键审核点。

```yaml
ReviewGate:
  id: string
  project_id: string
  type: product_intent | feature_scope | architecture | design | implementation_plan | release
  status: pending | waiting_human | approved | approved_with_changes | rejected | superseded
  title: string
  summary: string
  form_spec_id: string
  response_id: string
  created_by_agent: string
  approved_by: string
  created_at: timestamp
  resolved_at: timestamp
  linked_decisions: []string
  linked_tasks: []string
  linked_artifacts: []string
```

### 4.4 DynamicFormSpec

动态表单的 schema。可由 agent 生成，也可由 coder 内置模板生成。

```yaml
DynamicFormSpec:
  id: string
  gate_id: string
  title: string
  description: string
  fields:
    - id: string
      label: string
      type: text | textarea | select | multi_select | checklist | ranked_list | feature_matrix | table | diagram | artifact_review | annotation
      required: bool
      options: []string
      default: any
      help: string
  submit_actions:
    - approve
    - approve_with_changes
    - request_revision
    - reject
```

### 4.5 HumanReviewResponse

人的反馈必须持久化为结构化对象。

```yaml
HumanReviewResponse:
  id: string
  gate_id: string
  decision: approve | approve_with_changes | request_revision | reject
  answers: map
  freeform_comment: string
  reviewer: human
  created_at: timestamp
```

提交后应自动：

1. 记录 event。
2. 生成 Decision。
3. 更新 ContextPacket。
4. 唤醒或继续对应 team thread。
5. 将表单响应链接到后续任务和产物。

### 4.6 TaskGraph

任务必须持久化、可追溯。

```yaml
Task:
  id: string
  project_id: string
  title: string
  status: pending | in_progress | blocked | review | done | cancelled
  owner_agent: string
  dependencies: []string
  inputs:
    context_packet_version: int
    decisions: []string
    review_gates: []string
  outputs:
    artifacts: []string
    commits: []string
    test_evidence: []string
  created_at: timestamp
  updated_at: timestamp
```

## 5. 人审节点规划

### 5.1 产品意图确认

目的：把用户的一句话产品想法收束成明确方向。

Agents 产出：

- 产品目标。
- 目标用户。
- 核心场景。
- MVP 草案。
- 非目标。
- 风险和假设。

表单字段：

- 产品名。
- 目标用户。
- 核心问题。
- MVP 功能。
- 非目标。
- 优先级：最快可用 / 体验优先 / 技术稳健 / 商业验证。
- 审核动作：通过 / 修改后继续 / 重新生成。

### 5.2 产品功能确认

目的：确认功能范围和优先级。

表单字段：

- 功能矩阵：include、priority、reason。
- 必须有 / 可选 / 暂缓。
- 用户补充功能。
- 是否允许 agents 主动提出替代功能。

输出：FeatureScope Decision。

### 5.3 架构确认

目的：确认技术方向、部署方向和风险偏好。

表单字段：

- 技术栈摘要。
- 架构图。
- 数据模型摘要。
- 部署目标。
- 风险偏好：简单优先 / 稳定优先 / 扩展优先。
- 明确禁止项，例如不要 Kubernetes、不要复杂权限。

输出：Architecture Decision Record。

### 5.4 设计稿确认

目的：让用户从真实体验角度校准方向。

表单字段：

- 用户流程预览。
- 页面列表。
- 设计稿或线框图。
- 页面级反馈。
- 风格选择。
- 文案和交互备注。

输出：Design Decision 和 UI tasks。

### 5.5 开发计划确认

目的：确认里程碑、任务顺序和并行策略。

表单字段：

- Milestone list。
- 任务拆解。
- 风险表。
- 是否允许并行 agents。
- 是否缩小范围。

输出：TaskGraph v1。

### 5.6 上线前确认

目的：防止黑盒自动上线。

表单字段：

- 测试摘要。
- 已知问题。
- 部署目标。
- 环境变量检查。
- 回滚方案。
- 审核动作：部署上线 / 先修问题 / 仅生成部署包。

输出：Release Decision。

## 6. 用户故事

### 6.1 从一个想法开始

用户输入：

```bash
tt coder start "我想做一个帮助小团队筛简历的工具"
```

系统行为：

1. 创建 CoderProject。
2. 创建 ContextPacket v1。
3. 启动产品、架构、设计方向的初步 team 探索。
4. 生成产品意图确认表单。
5. dashboard 展示等待用户确认。

用户看到：

- 产品目标草案。
- 目标用户草案。
- MVP 功能建议。
- 非目标建议。
- 可编辑表单。

### 6.2 用户参与功能取舍

用户在功能表单中修改：

- 登录系统：暂缓。
- 团队权限：暂缓。
- 简历上传、解析、候选人列表：P0。
- 候选人评分：P1。

系统行为：

1. 保存 HumanReviewResponse。
2. 生成 FeatureScope Decision。
3. 更新 ContextPacket。
4. 后续 agents 不得擅自实现登录和权限。

### 6.3 用户不懂技术但审核架构

系统展示两个方案：

- A：单体 Go + React + SQLite，最快可用。
- B：Go API + React + Postgres + Docker Compose，更适合上线。

用户选择：

- Docker Compose。
- Postgres。
- 简单但可上线。
- 不要 Kubernetes。

系统行为：

- 生成 Architecture Decision。
- 更新部署约束。
- 通知 implementer、test-engineer、devops。

### 6.4 用户审核设计稿

用户在设计稿表单中反馈：

- 候选人评分不要太突出。
- 上传后允许查看原始简历。

系统行为：

- 设计 agent 修改页面。
- 产品 agent 更新需求。
- 开发任务增加“原始简历查看”。
- 所有变化链接到设计审核记录。

### 6.5 用户查看开发进度

用户运行：

```bash
tt coder status
```

或打开：

```bash
tt coder open
```

看到：

- 当前阶段。
- 当前 gate。
- Agents 状态。
- TaskGraph。
- Blockers。
- 最近决策。
- 测试证据。

### 6.6 用户处理开发中阻塞

系统提出阻塞问题：

> 是否支持 DOCX？

表单选项：

- 本版本支持。
- 下一版本支持。
- 不支持。

用户选择“下一版本支持”。

系统行为：

- 记录 decision。
- 更新 non_goals / later scope。
- unblock 当前 PDF-only 实现。

### 6.7 用户上线前审核

系统展示：

- 12 个测试通过。
- 已知限制：仅支持 PDF。
- 部署目标：Docker Compose。
- 回滚方式：保留旧镜像。
- 数据库迁移：无破坏性迁移。

用户选择：

- 部署上线。

系统行为：

- 记录 Release Decision。
- 执行部署。
- 记录 deployment artifact。
- 更新 project status = live。

## 7. 数据落盘建议

第一版建议沿用 `.tt/`：

```text
.tt/coder/
  projects/
    <project-id>/
      project.json
      context/
        v0001.json
        v0002.json
      gates/
        <gate-id>/
          form.json
          response.json
      decisions.jsonl
      tasks.jsonl
      artifacts.jsonl
      threads.json
```

说明：

- `context/v*.json` 是版本化上下文快照。
- `decisions.jsonl` 是 append-only 决策日志。
- `tasks.jsonl` 是任务状态事件流或快照事件。
- `artifacts.jsonl` 记录设计稿、代码 diff、测试报告、部署记录。
- `threads.json` 映射到底层 `tt team` thread。

## 8. 与现有系统的集成方式

### 8.1 与 team 集成

`coder` 可以将 ContextPacket 渲染为 team 的当前用户问题前缀，或扩展 team runtime 支持结构化 context section。

第一版推荐低侵入方案：

```text
[CODER_CONTEXT_PACKET]
{...json...}
[/CODER_CONTEXT_PACKET]

当前阶段任务：...
```

后续再升级为 team runtime 的一等字段。

### 8.2 与 blackboard 集成

短期协作状态继续放 blackboard：

- open questions。
- blockers。
- agent findings。
- implementation artifacts。

但关键人审结果必须进入 coder decision log，不能只留在 blackboard。

### 8.3 与 memory 集成

长期稳定信息可以进入 team memory：

- 用户偏好。
- 稳定架构决策。
- 团队工作约定。

但当前任务状态、未完成事项、临时 gate 不应写入长期 memory。

### 8.4 与 formula dynamic form 集成

短期可复用 formula 的 HumanInputRequest / response validation / dashboard submission 思路。

推荐抽象：

```text
ReviewGate -> DynamicFormSpec -> HumanReviewResponse -> Resume team/coder workflow
```

如果实现成本允许，可以把 formula 的 form renderer 抽成共享 UI 包。

## 9. MVP 命令规划

### 9.1 `tt coder start`

```bash
tt coder start "我想做一个 AI 招聘助手"
```

职责：

- 创建项目。
- 创建 ContextPacket v1。
- 创建初始 team thread。
- 生成产品意图 ReviewGate。
- 输出 dashboard URL 或下一步命令。

### 9.2 `tt coder status`

展示：

- project status。
- current phase。
- current gate。
- waiting human action。
- active agents。
- blockers。
- recent decisions。

### 9.3 `tt coder open`

打开 dashboard：

- 动态表单。
- ContextPacket 摘要。
- TaskGraph。
- DecisionLog。
- Team discussion。
- Artifacts。

### 9.4 `tt coder approve`

CLI 审批入口：

```bash
tt coder approve <gate-id> --decision approve_with_changes --set deployment=DockerCompose --comment "先不做权限"
```

职责：

- 校验 gate 状态。
- 保存 response。
- 更新 context。
- resume 当前 workflow。

### 9.5 `tt coder show`

展示完整追溯：

- 当前上下文版本。
- 决策历史。
- 任务图。
- 审核记录。
- 产物和证据。

## 10. 分阶段实施计划

### Phase 0：规划落档

目标：先形成本文档，作为后续实现上下文。

验收：

- 文档存在于 `ai-docs/coder-product-development.md`。
- 文档列出现有能力复用、核心 schema、人审 gate、用户故事和实施阶段。

### Phase 1：只读 schema 与本地 store

目标：实现 coder project 的数据模型和本地持久化。

候选包：

```text
internal/coder/
  project.go
  context.go
  gate.go
  form.go
  store.go
```

验收：

- 能创建 CoderProject。
- 能保存 / 加载 ContextPacket。
- 能保存 ReviewGate、DynamicFormSpec、HumanReviewResponse。
- 有单元测试覆盖 JSON round-trip、版本递增、缺失文件错误。

### Phase 2：CLI 骨架

目标：新增 `tt coder` 命令。

命令：

- `tt coder start <idea>`
- `tt coder status [project]`
- `tt coder show [project]`
- `tt coder approve <gate>`

验收：

- CLI help 可见。
- start 能创建项目和第一个 product_intent gate。
- status 能显示 waiting gate。
- approve 能写入 response 并更新 context。

### Phase 3：复用动态表单 UI

目标：在 dashboard 展示 ReviewGate 表单。

实现策略：

- 先做 coder dashboard 的 `/api/state` 与 `/api/review-response`。
- 表单 renderer 尽量复用 formula dashboard 现有结构。
- 支持 text、textarea、select、checklist、ranked_list、feature_matrix 的最小集合。

验收：

- 用户可在浏览器提交 product_intent gate。
- response 持久化。
- context packet 更新。
- 事件可追溯。

### Phase 4：team 集成

目标：把 ContextPacket 注入 `software-development` 或 coder 专用 team。

验收：

- `tt coder start` 能启动或准备 team thread。
- team prompt 中包含 ContextPacket。
- team 输出可生成下一个 ReviewGate。
- 用户审批后可 resume。

### Phase 5：任务图和产物追溯

目标：建立 TaskGraph、DecisionLog、ArtifactRegistry。

验收：

- 每个任务能关联输入 context、review gate、decision、artifact、test evidence。
- dashboard 能按任务展示证据链。
- `tt coder show` 能追溯“为什么这样做”。

### Phase 6：上线 gate

目标：支持 release review。

验收：

- 测试证据和部署计划进入 release gate。
- 用户批准前不得执行部署。
- 部署结果和回滚信息持久化。

## 11. 风险与约束

### 11.1 避免过早重构 team runtime

第一版应通过 ContextPacket 文本/JSON 注入复用 team，避免大改 runtime。

### 11.2 避免动态表单系统分裂

formula 已有 dynamic form。coder 应复用 schema 思路和 UI 组件，后续可抽共享包。

### 11.3 避免 memory 污染

当前任务状态不进入 durable memory。只有稳定偏好、稳定架构决策和团队约定可进入 memory。

### 11.4 避免人审阻塞过多

默认只在关键 gate 需要人审。小问题可由 agents 做合理假设并记录。

### 11.5 避免黑盒上线

上线、支付、删除数据、不可逆操作必须有明确人审。

## 12. 第一阶段建议切入点

最小可交付不是马上让 agents 自动开发产品，而是先建立可持久化的 coder 项目骨架。

建议下一个实现任务：

> 实现 `internal/coder` 的 Project、ContextPacket、ReviewGate、DynamicFormSpec、HumanReviewResponse 和本地 Store，并补单元测试。

原因：

- 不依赖 LLM。
- 不碰复杂 UI。
- 不破坏现有 team/runtime。
- 为 CLI、dashboard、team 集成提供稳定基础。

完成后再加 `tt coder start/status/show/approve`。
