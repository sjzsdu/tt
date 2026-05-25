# Formula 工作流系统

`tt` 的 formula 子系统是整个仓库里最值得单独理解的部分。它已经不只是“模板生成器”，而是一个带有编译、执行、持久化和可视化能力的小型工作流引擎。

## 先给结论

如果要用一句话概括它：

> Formula 先把声明式工作流定义编译成 graph-first typed runtime `Workflow`，再执行。typed runtime 负责步骤调度、上下文、人工输入、resume/retry，`formularun` 负责把状态、事件和产物持续落盘，dashboard 负责观察与交互。

## 系统总图

```mermaid
flowchart TD
    A[formula TOML / JSON] --> B[Parser]
    A0[Curated builtin formulas] --> B
    B --> C[Resolve]
    C --> D[Compile]
    D --> E[Workflow]
    E --> W[Typed Workflow]
    W --> F[internal/formula/runtime]
    F --> G[Agent Steps]
    F --> H[Script Steps]
    F --> I[Runtime Context]
    F --> X{需要人工输入?}
    X -->|否| J[Run Store]
    X -->|是| Y[waiting_input]
    Y --> Z[CLI / Dashboard Form]
    Z --> F
    J --> K[run.json / workflow.json / state.json / logs.jsonl]
    J --> L[steps/*.prompt output error]
    F --> M[Dashboard]
```

读这张图时，可以把 formula 理解成四段链路：

1. **定义阶段**：写 formula 文件
2. **编译阶段**：把定义变成稳定的 Workflow 图
3. **执行阶段**：typed runtime 按依赖图运行 agent/script/human_input 步骤
4. **观察阶段**：通过 run store 和 dashboard 回看或恢复运行

## Curated builtin formula

系统保留少量经过筛选的内置 formula，而不是维护一个大而泛的低质量 catalog。当前内置：

- `fresh-topic-docs`：根据用户输入的新鲜事物、概念、趋势或话题，生成一组 Markdown 系列文档，包括快速入门、背景脉络、核心机制、应用影响、争议风险、学习路径和问题清单。

常用命令：

```bash
tt formula list --builtin
tt formula show fresh-topic-docs
tt formula run fresh-topic-docs "空间计算" --dry-run
tt formula copy fresh-topic-docs .tt/formulas/fresh-topic-docs.toml
```

## Formula 定义模型

核心定义在 `internal/formula/types.go`。

### 顶层对象 `Formula`

它包含：
- `formula`：名称
- `description`
- `version`
- `type`：`workflow` / `expansion` / `aspect`
- `vars`
- `steps`
- `template`
- `compose`
- `advice`
- `pointcuts`
- `phase`
- `pour`

这里最重要的不是字段数量，而是它说明了：**formula 不是只有“步骤列表”，还支持继承、扩展、切面和组合。**

### 步骤对象 `Step`

一个步骤可以携带：
- `id` / `title` / `description`
- `depends_on` / `needs`
- `condition`
- `children`
- `gate`
- `loop`
- `retry`
- `agent`
- `script`
- step `id` 输出键
- `input_context`
- `execution`
- `form`

也就是说，step 已经是一个相当丰富的工作单元定义，而不是简单字符串任务。其中 `form` 用于描述人工介入时要展示给用户的表单。

## 编译前后：为什么要有 `Workflow`

源码中一个关键设计是：**执行器不直接跑 `Formula`，而是跑 `Workflow`。**

### 编译前：面向作者的 Formula
- 支持继承、扩展、切面、变量等高层表达
- 更适合手写和维护

### 编译后：面向执行器的 Workflow
- `Steps` 已扁平化
- `Deps` 已显式化
- start/end 边界已补齐
- 可直接做拓扑排序和批次执行

```mermaid
flowchart LR
    A[Formula\n作者视角] --> B[Compile]
    B --> C[Workflow\n执行器视角]

    A1[extends / advice / expand / vars / children] --> A
    C --> C1[扁平步骤列表]
    C --> C2[显式依赖边]
    C --> C3[start/end 边界]
```

这张图说明：Workflow 的价值不只是“中间格式”，而是把高层定义降维成执行期更稳定的数据结构。

## 编译阶段做了什么

`internal/formula/compile.go` 的主线大致是：

1. `LoadByName` 载入 formula
2. `Resolve` 处理继承与组合
3. 校验变量
4. 准备 compile-time 变量
5. `ApplyControlFlowWithVars`
6. `ApplyAdvice`
7. `ApplyInlineExpansionsWithVars`
8. `ApplyExpansionsWithVars`
9. `FilterStepsByCondition`
10. `toWorkflow`

## 编译阶段示意图

```mermaid
flowchart TD
    A[Load formula] --> B[Resolve extends / compose]
    B --> C[Validate vars]
    C --> D[Apply control flow]
    D --> E[Apply advice]
    E --> F[Apply inline expansions]
    F --> G[Apply compose expansions]
    G --> H[Filter compile-time conditions]
    H --> I[toWorkflow]
    I --> J[Add start/end boundary steps]
```

这张图回答的是：**一个 formula 文件到底怎样一步步变成可执行图。**

## 边界步骤：start / end 的设计意义

源码会在 `toWorkflow()` 之后调用 `addWorkflowBoundarySteps()` 自动补齐：

- `<formula>.start`
- `<formula>.end`

如果用户没显式定义，它们会以 `noop` 边界步骤出现。

这样做的好处有三个：

1. 所有图都有明确入口和出口。
2. 没有前驱/后继的真实步骤可以统一挂接。
3. Mermaid 图和 dashboard 展示会更稳定。

## 执行模型：DAG 分批执行

typed runtime 在 `internal/formula/runtime` 中执行 `Workflow` 图，再做拓扑规划：

- 同一批次：没有互相依赖，可以并发执行
- 下一批次：等待前一批完成后再执行

```mermaid
flowchart TD
    S[start] --> A[collect-context]
    A --> B1[plan-frontend]
    A --> B2[plan-backend]
    B1 --> C[review]
    B2 --> C
    C --> E[end]
```

从这类图里，你应该读出的是：**typed runtime 的执行单位不是随机调度，而是基于显式依赖图规划出的可运行节点。**

## 运行时上下文：step `id` 与 `input_context`

formula 的运行时数据流主要靠两组字段：

- step `id` 输出键：把某一步输出写入上下文
- `input_context`：后续步骤把指定 key 的内容注入 prompt

typed runtime 在 step 完成后，会默认把输出写入以 step `id` 命名的 `ContextStore` key。后续步骤可通过 `input_context`、runtime condition 或 loop.until 读取这些输出。

```mermaid
flowchart LR
    A[decide 输出] -->|key=decide| C[(runtime context)]
    C -->|input_context: decide| B[Step B Prompt]
    C -->|condition: decide.path == frontend| D[是否执行后续分支]
```

这张图说明，formula 的数据流不是靠全局共享对象硬编码，而是靠显式命名的上下文键连接起来的。

## 条件执行：compile-time 与 runtime 的区别

这是这个子系统里非常值得先搞清楚的一点。

### compile-time condition
在编译期基于变量做过滤，步骤可能直接不进入最终 Workflow。

### runtime condition
在执行期根据上下文判断要不要跳过某个步骤。

typed runtime 使用 formula 表达式能力在执行期评估 condition，支持：
- `==`
- `!=`
- `=~`
- `&&`
- `||`
- JSON path 风格访问，如 `decide.path` 或 `frontend-plan.ready`

## 循环：`loop.until` 的运行时语义

当步骤带有 `loop.until` 时，typed runtime 会按 loop 语义执行 body，并在每轮后评估退出条件。

### loop 的特点
- 运行时循环
- 每次迭代都可执行 body steps
- body step 可以是 agent step，也可以是 script step
- 使用 `{{iteration}}` 渲染标题/描述
- 每轮结束后评估 `until`
- 达到 `max` 后停止

```mermaid
flowchart TD
    A[进入 loop step] --> B[执行 body step 1]
    B --> C[执行 body step 2]
    C --> D[将 body 输出写入 context]
    D --> E{until 条件成立?}
    E -- 是 --> F[loop 完成]
    E -- 否 --> G{达到 max?}
    G -- 否 --> B
    G -- 是 --> H[以 max 达成方式结束]
```

这里最重要的理解是：**loop 不是编译期展开 N 份步骤，而是执行期控制一个循环体。**

## agent step 与 script step

formula 支持两种主要执行方式。

### 1. agent step
- 默认模式
- typed runtime/step adapter 负责组装运行请求
- 通过 Picoclaw `DirectRunner` 调用 agent
- 支持 step 级 agent/model/session 覆盖

### 2. script step
- `execution == "script"`
- 使用 `ScriptSpec` 执行本地命令
- 支持 argv 形式命令
- shell 默认禁用，需显式允许
- 有命令 denylist 和危险模式拦截
- 可声明 stdout 必须是 JSON

```mermaid
flowchart LR
    A[Workflow node] --> B{execution}
    B -- script --> C[executeScriptStep]
    B -- other --> D[buildPrompt + runner]
    C --> E[stdout/stderr/json/duration]
    D --> F[agent response]
    E --> G[写入 step-id context]
    F --> G
```

这张图解释的是：**formula 执行器不是纯 LLM 调度器，它也能跑可控的本地确定性步骤。**

### 3. human input step

当步骤声明 `execution = "human_input"` 时，它不会调用 agent 或 script，而是把运行挂起到 `waiting_input`，并把步骤里的 `form` 保存为待填写请求。

```toml
[[steps]]
id = "choose-direction"
title = "选择方向"
execution = "human_input"

[steps.form]
title = "请选择继续方向"
description = "用户提交后，工作流会从后续步骤继续。"

[[steps.form.fields]]
name = "direction"
label = "方向"
type = "radio"
required = true
options = ["技术", "产品", "市场"]
```

支持的字段类型：`input`、`textarea`、`radio`、`checkbox`、`select`。

## 人工介入：静态 Form 与动态澄清

人工介入有两种入口：

1. **静态入口**：formula 作者在 TOML 里写 `execution = "human_input"` 和 `[steps.form]`。适合工作流中明确需要用户选择、确认或补充材料的节点。
2. **动态入口**：agent 在执行过程中发现缺少关键信息，可以输出 fenced block：

````markdown
```tt-human-input
{
  "reason": "需要用户确认目标受众，否则后续文章结构会偏离。",
  "form": {
    "title": "确认目标受众",
    "fields": [
      {"name":"audience","label":"目标受众","type":"input","required":true}
    ]
  }
}
```
````

执行器会解析这段 JSON，将当前 run 标为 `waiting_input`，并保存请求。用户提交后，该步骤会以提交内容作为输出完成，后续 DAG 继续运行。

### 提交方式

CLI：

```bash
tt formula run input latest choose-direction --field direction=技术
```

Dashboard：

- live run dashboard 会自动弹出表单。
- 用户提交后服务端保存响应、标记等待步骤完成，并继续执行剩余步骤。

### 持久化文件

人工介入会额外写入：

```text
steps/<step>.human_input_request.json
steps/<step>.human_input_response.json
steps/<step>.output.md
```

其中 `output.md` 是提交内容的 JSON 表示，便于后续步骤通过 `input_context` 消费。

## 运行持久化：为什么它很重要

没有 `internal/formularun`，formula 只能算一次性执行；有了它，formula 才更像一个真正的工作流系统。

每次运行会生成目录：

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

### 这些文件分别回答什么问题

| 文件 | 回答的问题 |
| --- | --- |
| `run.json` | 这次运行是谁、何时、以什么参数启动的？ |
| `workflow.json` | 当时真正执行的 Workflow 长什么样？ |
| `state.json` | 当前/最终状态快照是什么？ |
| `logs.jsonl` | 执行过程中发生了哪些事件？ |
| `steps/*` | 每一步到底给了什么 prompt，得到了什么输出？ |

## 运行状态流转

```mermaid
stateDiagram-v2
    [*] --> running
    running --> completed
    running --> failed
    running --> interrupted
    running --> stale
    running --> waiting_input
    waiting_input --> running
```

`waiting_input` 表示工作流主动挂起，等待用户通过 CLI 或 dashboard 提交表单。`stale` 表示一个 run 还标记为 running，但对应进程已经不存在，系统会把它标成 stale。这说明 run store 已经在承担“运行恢复”和“状态清理”的职责。

## Dashboard 的角色

formula 不只是把结果写盘，还会通过 dashboard 展示：

- 哪些步骤在运行
- 哪些已完成/失败
- 哪些在等待人工输入
- 日志和输出
- 人工输入表单与提交后继续执行
- Workspace 关联目录

因此 dashboard 不只是一个 UI 点缀，而是把“执行时可观察性”补齐的重要一环。

当前需要注意：实时运行中的 dashboard 可以提交人工输入；通过 `tt formula run open` 打开的历史只读 dashboard 主要用于查看，不能直接恢复提交，需使用 CLI input 或重新启动可写运行服务。

## create / optimize：让 formula 可以被 agent 生产

`cmd/formula/` 子包里还有两个非常有代表性的命令：

- `formula create`
- `formula optimize`

它们通过内嵌 `formula-writer` agent：
- 根据自然语言请求生成 TOML
- 本地再次做 parse + validate
- optimize 失败时还能触发一次 repair 流程

这让 formula 系统不只是“运行工作流”，还具备了“让 agent 设计工作流”的能力。

## 读源码时最值得优先看的链路

如果你想快速进入主线，建议按下面顺序读：

1. `internal/formula/types.go`
2. `internal/formula/compile.go`
3. `internal/formula/workflow.go`
4. `internal/formula/runtime/executor.go`
5. `internal/formula/steps/`
6. `internal/formularun/store.go`
7. `internal/formulaui/`、`internal/formuladoc/`
8. `cmd/formula/`

## 实现参考

- 定义模型：`internal/formula/types.go`
- 编译入口：`internal/formula/compile.go`
- Workflow 生成：`internal/formula/workflow.go`
- 条件执行与恢复：`internal/formula/runtime/executor.go`
- Step 实现：`internal/formula/steps/`
- 运行持久化：`internal/formularun/store.go`
- 命令编排：`cmd/formula/`，入口注册在 `cmd/formula.go`
- Dashboard 状态：`internal/formulaui/`，服务 glue 在 `cmd/formula/`
- 示例：`examples/formulas/*`
