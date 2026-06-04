# Formula 工作流系统

> 最后更新：2026-06-02

`tt formula` 现在是一个 graph-first typed workflow 引擎。它不再通过旧的 task tree 中间模型执行，而是把声明式 TOML 直接解析、展开并编译成 `internal/formula/ir.Workflow`，再由 typed runtime 执行具体的 `steps.Step` 实现。

## 一句话结论

> Formula = TOML 定义 + Workflow IR 编译 + Step 接口实现 + typed runtime 调度 + run store/dashboard 可观察性。

## 系统总图

```mermaid
flowchart TD
    A[formula TOML / JSON] --> B[Parser]
    A0[internal/formula/builtin<br/>formulas + atomics] --> B
    B --> C[Resolve extends / compose / embeds / expand / advice]
    C --> D[ApplyControlFlow / FilterStepsByCondition]
    D --> E[compile/compiler.go<br/>AST → IR.Workflow]
    E --> F[typed runtime Executor]
    F --> P[preflight checks]
    P --> W[prepareWorkspace worktree]
    W --> Topo[PlanTopological]
    Topo --> Step{Step kind}
    Step -->|agent| G[AgentStep + Capabilities.Agents]
    Step -->|script| H[ScriptStep + ScriptCapability<br/>含 safety policy + coder repair]
    Step -->|human_input| I[HumanInputStep]
    Step -->|aggregate| AA[AggregateStep]
    Step -->|tool / write_files| TT[ToolStep / WriteFilesStep]
    Step -->|loop / retry / noop| LL[Loop / Retry / Noop]
    F --> X{waiting input?}
    X -->|no| J[Run Store]
    X -->|yes| Y[CLI / Dashboard Form]
    Y --> F
    J --> K[run.json / workflow.json / state.json / logs.jsonl]
    J --> L[steps/*.prompt output error / human_input]
    F --> M[Dashboard]
```

五段链路：

1. **定义阶段**：作者写 formula TOML（用户目录或 `internal/formula/builtin/`）。
2. **编译阶段**：解析继承、组合、扩展、嵌入、advice、compile-time condition；用 `internal/formula/compile/compiler.go` 把 AST 编译成 `ir.Workflow`，节点上挂 typed `steps.Step`。
3. **预检与 workspace 准备**：`preflight` 检查 + `prepareWorkspace`（按 `WorkspaceSpec` 创建 worktree/分支/sparse checkout）。
4. **执行阶段**：typed runtime 按依赖图运行具体 Step 实现。包含 runtime condition、loop/until、output validation advice、script repair。
5. **观察阶段**：run store + dashboard 持续展示、恢复、提交人工输入。

## 定义模型

核心源文件：`internal/formula/types.go`。

顶层 `Formula` 包含：

- `formula` / `description` / `title` / `category` / `tags` / `version` / `type` / `contract`
- `vars`（变量声明）
- `steps` / `template`（steps 与 expansion template）
- `compose`（bond_points / hooks / expand / map / branch / gate / aspects）
- `advice` / `pointcuts`
- `extends`
- `phase` / `pour` / `worktree` / `workspace`（执行策略）
- `preflight`（预检）
- `source`（由 parser 填入，运行时回溯来源）

一个 `Step` 可以声明：

- `id` / `title` / `description` / `description_file` / `notes`
- `type` / `priority` / `labels` / `metadata` / `assignee`
- `depends_on` / `needs` / `waits_for`
- `condition`（compile-time 与 runtime 共用，runtime 时支持 `==`/`!=`/`=~`）
- `children`（子步骤）
- `gate` / `loop` / `on_complete` / `retry` / `timeout`
- `agent` / `script` / `aggregate` / `tool` / `write_files` / `form` / `dynamic_form`
- `validate`（output schema）
- `output_key`（可选；推荐用 step `id`）
- `input_context`
- `execution`（`agent` / `script` / `human_input` / `aggregate` / `tool` / `write_files` / `noop`）
- `expand` / `expand_vars` / `embed` / `embed_vars`

当前推荐：用 step `id` 作为输出上下文 key。普通公式不要再写 `output_key`。

## 编译模型：Formula 到 Workflow

源码入口：`internal/formula/workflow.go`。

主线是：

1. `LoadByName`
2. `Resolve`
3. runtime vars 校验
4. compile-time vars 校验
5. `ApplyControlFlowWithVars`
6. `ApplyAdvice`
7. `ApplyInlineExpansionsWithVars`
8. `ApplyExpansionsWithVars`
9. `ApplyEmbedsWithVars`
10. `FilterStepsByCondition`
11. `WorkflowFromFormula`

```mermaid
flowchart TD
    A[Load formula] --> B[Resolve extends / compose]
    B --> C[Validate vars]
    C --> D[Apply control flow]
    D --> E[Apply advice]
    E --> F[Apply inline expansions]
    F --> G[Apply compose expansions]
    G --> H[Apply embeds]
    H --> I[Filter compile-time conditions]
    I --> J[WorkflowFromFormula]
    J --> K[Workflow IR]
```

`Workflow` 是执行器视角的数据结构：

- `ID` / `Name` / `Description`
- `Vars`
- `Graph.Nodes`
- `Graph.Edges`

每个 node 持有一个 `steps.Step` 接口实例，而不是旧的扁平任务结构。

## Step 接口与实现

核心接口：`internal/formula/steps/step.go`。

```go
type Step interface {
    Meta() Metadata
    Validate(ValidationContext) error
}

type Executable interface {
    Step
    Run(context.Context, RunRequest) (*RunResult, error)
}
```

现有实现集中在 `internal/formula/steps/kinds.go`：

- `NoopStep`
- `AgentStep`（含 `DynamicForm` 协议注入 + `Formula step execution guard` 抗漂移）
- `ExternalAgentStep`
- `ScriptStep`（通过 `Capabilities.Scripts` 执行，注入 `env`）
- `HumanInputStep`（静态 Form）
- `LoopStep`（嵌套 typed step，支持 parallel / max_concurrency / for_each / until / range）
- `RetryStep`（attempt / on_exhausted）
- `AggregateStep`（基于已有 context 的投影/收集）
- `ToolStep`（deterministic 内建工具：write_files / sleep / git_*）
- `WriteFilesStep`（按 JSON 字段写入文件）

其中 agent/external_agent/script/human_input/noop 已经是直接运行路径，Loop/Retry/Aggregate/Tool/WriteFiles 是面向扩展的 typed step 结构。

`ExternalAgentStep` 用于把某一步交给外部 agent CLI 执行，内置 driver 包括 `jcode`、`codex`、`opencode`、`forge`、`bl`。旧版 recipe TOML 写法：

```toml
[[steps]]
id = "external-review"
execution = "external_agent"
description = "Review the current diff and return risks."
input_context = ["collect-diff"]

[steps.external_agent]
driver = "codex"
model = "gpt-5"
mode = "exec"
timeout = "5m"
extra_args = ["--sandbox", "read-only"]
```

新版 typed schema 写法使用 `kind = "external_agent"`，并把 driver 字段放在 step 顶层：

```toml
[[steps]]
id = "external-review"
kind = "external_agent"
prompt = "Review the current diff and return risks."
input_context = ["collect-diff"]
driver = "codex"
model = "gpt-5"
mode = "exec"
timeout = "5m"
```

注意：`codex` 的 resume 通过 `codex exec resume <session>`；`forge` 使用顶层 `forge --prompt` / `--conversation-id`，当前不注入 `--model`，默认使用 forge 安装/配置时已调配好的 provider 和 model。

外部 CLI 依赖建议用 conditional preflight 表达，`condition` 使用 compile-time 变量语法：

```toml
[[preflight.checks]]
type = "command"
name = "codex"
command = "codex"
condition = "{{driver}} == codex"
message = "driver=codex 时需要安装 Codex CLI。"
```

`Step` 的 kind 枚举（`internal/formula/steps/step.go`）：

```go
const (
    KindNoop          Kind = "noop"
    KindAgent         Kind = "agent"
    KindExternalAgent Kind = "external_agent"
    KindScript        Kind = "script"
    KindHumanInput    Kind = "human_input"
    KindLoop          Kind = "loop"
    KindCondition     Kind = "condition"
    KindGate          Kind = "gate"
    KindRetry         Kind = "retry"
    KindEmbed         Kind = "embed"
    KindExpand        Kind = "expand"
    KindTool          Kind = "tool"
    KindAggregate     Kind = "aggregate"
    KindWriteFiles    Kind = "write_files"
)
```

更详细的 step kind 写法、字段含义和示例见 [step-kinds-reference.md](./step-kinds-reference.md)。

## 运行模型

核心执行器：`internal/formula/runtime/executor.go`。

执行器做的事：

1. 调用 `e.Store.StartWorkflow` 并触发 `workflow.started` 事件。
2. 调用 `prepareWorkspace`（按 `WorkspaceSpec` 创建 worktree / 分支 / 稀疏 checkout），并触发 `workflow.workspace.ready` 事件。
3. 调用 `PlanTopological` 得到执行顺序。
4. 对每个 node：先恢复上次已完成 step 的结果；用 `shouldRunStep` 评估 runtime condition；用对应 `Step` 实现执行；按 status 处理：
   - `completed` → 把输出写入 `ContextStore`（key = step id 或 `output_key`）+ `SaveStep` + `step.completed`
   - `waiting` → 标记 `waiting_input` 并停机
   - `failed` → 先尝试 `tryRepairFailedScriptStep`（用 `coder` agent 修一次）；否则终止
   - output 校验失败 → 若是 agent step，附加 advice retry 一次
5. 结束时 `FinishWorkflow` + `workflow.completed`/`failed` 事件；并 `finalizeWorkspace`（按 cleanup 策略删除临时 worktree）。

```mermaid
flowchart TD
    A[Workflow Graph] --> B[prepareWorkspace]
    B --> C[PlanTopological]
    C --> D[Node Ready]
    D --> E{Step Kind}
    E -->|agent| F[AgentRunner]
    E -->|script| G[ScriptRunner + safety policy]
    E -->|human_input| H[AwaitRequest]
    E -->|aggregate / tool / write_files| I[Aggregate / Tool / WriteFilesRunner]
    E -->|noop| J[completed]
    F --> K[ContextStore key = step id]
    G --> K
    G -. script 失败 .-> R[coder agent 修一次]
    F -. 校验失败 .-> Adv[advice retry]
    H --> L[waiting_input]
    K --> M[FormulaRunStateStore]
    Adv --> F
```

## 数据流：step id 就是输出 key

运行时默认把步骤输出写到以 step `id` 命名的 context key。后续步骤用：

```toml
input_context = ["fetch-pr", "review-risk"]
condition = "classify.kind == frontend"
```

不要为新 formula 写 `output_key`。稳定、语义化的 step id 就是最好的上下文键。

`ContextStore` 还会自动注入 `env` 键，包含 `cwd` / `invocation_cwd` / `workspace_cwd` / `formula_run_dir` / `os.{name,arch}` / `git.{is_repo,root,repo,branch,commit,remote_url}`。可以在 `description` / `prompt` 中通过 `{{env.git.branch}}` 等模板引用，也可以在 `condition` 中使用 `env.git.is_repo == true` 这样的判断（注意 `true` 在 `=`/正则上下文会被序列化为字符串 `"true"`，请用 `==` 显式写出）。

## 预检（preflight）

Formula 可以在 `[preflight]` 段声明启动前必过的检查，类型支持：

- `command` —— 仅检查可执行文件是否存在（不做调用）
- `exec` —— 真正执行命令验证可用性
- `git` —— 检查 `require_repo` / `require_remote` 等状态
- `env` —— 检查环境变量是否存在
- `path` —— 检查文件 / 目录是否存在

`bug-detect` 与 `github-pr-review` 等内置 formula 都依赖 preflight；缺失必要 CLI（`jira` / `gh`）会立即终止运行并提示安装。

## 人工输入模型

### 动态澄清，推荐用于不确定缺失信息

agent step 可声明：

```toml
form = true
```

runtime 会向 agent prompt 注入 `tt-human-input` 协议。如果 agent 输出该 fenced block，当前 step 进入 `waiting_input`，用户提交后该 step 以提交内容作为输出继续。

### 静态 human_input，只用于确定存在的用户门禁

```toml
[[steps]]
id = "choose-option"
execution = "human_input"

[steps.form]
title = "Choose an option"

[[steps.form.fields]]
name = "option"
label = "Option"
type = "radio"
required = true
options = ["safe", "fast", "complete"]
```

静态 form 适合审批、明确选项、必须由用户提供的私有上下文。不要把它当成默认澄清机制。

## 持久化模型

每次运行目录：

```text
.tt/runs/formula/<formula>/<run-id>/
  run.json
  workflow.json
  state.json
  logs.jsonl
  steps/
    <step>.prompt.md
    <step>.output.md
    <step>.error.txt
    <step>.human_input_request.json
    <step>.human_input_response.json
```

| 文件 | 用途 |
| --- | --- |
| `run.json` | 本次运行的元数据、状态、参数、工作区、session。 |
| `workflow.json` | 本次真正执行的 Workflow IR。 |
| `state.json` | runtime/dashboard 当前快照。 |
| `logs.jsonl` | 事件流。 |
| `steps/*` | 每个 step 的 prompt、输出、错误和人工输入文件。 |

## Dashboard

Dashboard 使用 `internal/formulaui` 的 UI DTO，从 `Workflow` 构建图展示。它负责：

- 展示步骤状态。
- 展示 prompt/output/error。
- 展示运行日志。
- 处理动态或静态人工输入表单。
- 支持失败步骤 retry。
- 打开历史 run 的只读视图。

## 命令入口

主要命令在 `cmd/formula/`：

- `tt formula list [--builtin] [--user] [--category ...]`
- `tt formula show <name> [--markdown]`
- `tt formula compile <name>`（输出 typed `ir.Workflow` 视角的快照）
- `tt formula validate <file>`
- `tt formula copy <name> [output]`
- `tt formula run <name> [positional]`
  - 旗标：`--agent`、`--model`、`--session`、`--web/--no-web`、`--web-port`、`--dry-run`、`--no-save`、`--no-script`、`--allow-shell-script`
  - 子命令：`open` / `show` / `rm` / `resume` / `input`
- `tt formula runs [--limit N] [--formula ...] [--status ...]`
- `tt formula create <name> <prompt...>`
- `tt formula optimize <name> <suggestion...>`

`compile` 输出的是 typed Workflow 视角；`run` 直接走 typed runtime；`create` / `optimize` 走嵌入式 `formula-writer` agent。

## 读源码顺序

1. `internal/formula/types.go`，作者侧语法模型。
2. `internal/formula/parser.go`，formula 解析。
3. `internal/formula/workflow.go`，Formula 到 typed `ir.Workflow` 的主编译链路（含 control flow / advice / embed / expand / filter）。
4. `internal/formula/ast/document.go` 与 `internal/formula/ir/workflow.go`，AST 与 IR。
5. `internal/formula/compile/compiler.go`，第二条 AST→IR 编译路径（用 Step Registry）。
6. `internal/formula/steps/step.go` + `registry.go` + `kinds.go`，Step 接口、注册表与所有 step 实现。
7. `internal/formula/runtime/executor.go`，运行时调度。
8. `internal/formula/runtime/environment.go` / `workspace.go` / `capabilities.go`，env / worktree / script safety。
9. `internal/formularun/store.go`，运行持久化。
10. `internal/formulaui/` / `internal/formuladoc/` / `internal/formularunview/`，展示和文档 DTO。
11. `cmd/formula/`，CLI 编排和 dashboard glue。

## 当前设计原则

- 不保留旧 task tree 兼容层。
- 用 Step 接口扩展新能力，而不是在一个大结构里继续堆字段。
- 新公式默认使用 step id 作为输出 key。
- 动态澄清优先于预设静态澄清表单。
- run store 保存 `workflow.json` 作为执行快照。
- agent step 强制注入 `Formula step execution guard`，避免 agent 只承认规则而不真做任务。
- script step 失败时自动用 `coder` agent 修一次，并把 `formula_repairs.<step-id>` 写入 context。
- output schema 校验失败时，对 agent step 自动追加 advice 重试一次。
- script step 永远走 `exec.CommandContext`，并默认拒绝 `rm -rf`、`shutdown`、`sudo` 等危险命令。
