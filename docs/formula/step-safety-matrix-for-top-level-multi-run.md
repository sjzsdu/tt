# 顶层多 formula 并发下的 step 类型安全矩阵（tt-ueo）

本文档聚焦 bead `tt-ueo`：在 **多个 formula 作为独立 run 并发存在** 的前提下，按 step kind 建立安全矩阵，区分哪些步骤可相对直接并发、哪些需要隔离前提、哪些应限制使用、哪些暂不纳入 MVP。

> 前置结论来自：
> - `docs/formula/current-run-model-and-top-level-concurrency-boundaries.md`（tt-iqj）：当前顶层执行单位仍是单 run，loop 内并发不能替代顶层多 run；
> - `docs/formula/workspace-and-side-effect-isolation-boundaries-for-multi-run.md`（tt-q3o）：共享 cwd 默认不安全，worktree 只解决 Git working tree 隔离，不覆盖全部副作用。

## 1. bead 在依赖树中的位置

- 当前 bead：`tt-ueo`，关注 **顶层多 formula 并发时，不同 step kind 的风险分层**。
- 上游依赖：
  - `tt-iqj` 已关闭：已确认当前 runtime / dashboard / resume 等核心行为都以单 run 为中心；
  - `tt-q3o` 已关闭：已明确 workspace 与副作用隔离边界。
- 下游阻塞：
  - `tt-2tr`：最小验证场景矩阵；
  - `tt-4ce`：MVP 方向与阶段边界。

因此本 bead 只回答：**当未来同时启动多个独立 formula run 时，不同 step kind 在并发维度上应被如何分层，以及判定依据是什么。**

## 2. 判定方法与矩阵标签

本文使用四档判定：

1. **可直接并发**
   - 顶层多 run 下通常不会主动写共享状态；
   - 不依赖额外隔离前提即可纳入第一阶段。

2. **需隔离条件**
   - 并发本身不是问题，但必须满足 workspace、输出路径、输入稳定性等前提；
   - 若缺失这些前提，不应视为安全。

3. **建议限制**
   - 从实现上可运行，但用户很容易误判风险；
   - MVP 更适合只在显式约束或人工控制下启用。

4. **暂不纳入 MVP**
   - 当前系统没有足够 guardrail，或 run-scoped / side-effect 风险过高；
   - 第一阶段不应默认承诺并发安全。

判定依据主要来自四个维度：

- **是否写共享文件系统或 Git 状态**；
- **是否触发 run-scoped 交互 / 修复 / 会话能力**；
- **是否可被 worktree 或 run-scoped 输出路径显著降险**；
- **是否仍会越过 workspace 去修改外部系统或共享资源**。

## 3. 总表

| step 类别 | 代表 kind / 能力 | 判定 | 主要依据 |
| --- | --- | --- | --- |
| `noop` | `noop` | 可直接并发 | 不执行副作用，只返回 completed |
| 只读 `aggregate` | `aggregate` | 可直接并发 | 仅对已有上下文做 JSON 投影，不写外部状态 |
| `sleep` 工具 | `tool.sleep` | 可直接并发 | 仅本 run 等待，不写共享状态 |
| `loop`（只读 body） | `loop` | 需隔离条件 | loop 自身只是容器，安全性取决于 body step |
| `retry` | `retry` | 需隔离条件 | retry 只是重试容器，风险取决于 child step 是否幂等 |
| `agent` | `agent` | 需隔离条件 | 默认 run-scoped，但可读写 workspace、生成动态表单、触发修复路径 |
| `external_agent` | `external_agent` | 需隔离条件 | 与 agent 类似，且外部 CLI/driver 行为更不可控 |
| 只读 `script` | `script` | 需隔离条件 | 若真正只读可并发，但脚本可轻易逃逸 workspace |
| `write_files` | `write_files` / `tool.write_files` | 建议限制 | 对共享输出路径敏感；当前无跨 run 冲突仲裁 |
| Git 确定性工具 | `tool.git_fetch` / `git_branch` / `git_checkout` / `git_worktree` / `git_push` | 建议限制 | 可比 shell git 更可见，但仍会改仓库/远端状态 |
| 写型 `script` | `script` | 建议限制 | 文件、缓存、端口、外部系统副作用不可由 runtime 统一兜底 |
| `human_input` | `human_input` | 暂不纳入 MVP | waiting / resume / submit 都是单 run 交互链路，聚合并发下操作归属复杂 |
| 动态 human input agent | `agent.dynamic_form = true` | 暂不纳入 MVP | agent 可能中途进入 waiting_input，聚合视图下人工响应成本高 |
| 自动修复 / repair | `StepFixer`、retry-step、repair confirm | 暂不纳入 MVP | repair 记录、确认、重跑均为 run-scoped，且对副作用 step 容易误重试 |
| final report chat / agent session | dashboard `/api/final-report-chat*`、`/api/agent-session` | 暂不纳入 MVP | 明确依赖单 run workspace、session 与最终报告上下文 |

## 4. 分类别说明

## 4.1 `noop`：可直接并发

对应：`steps.KindNoop`。

判定：**可直接并发**。

依据：
- `NoopStep.Run()` 直接返回 `completed`；
- 不读写 Git、文件系统、外部系统，也不进入 waiting / retry / repair。

已知不确定项：
- 几乎没有并发特定风险；
- 仅需注意它常被用作流程占位，不应被误读为“支持并发编排”的证据。

## 4.2 `aggregate`：可直接并发（前提是上游输入稳定）

对应：`steps.KindAggregate`。

判定：**可直接并发**。

依据：
- 该 step 只从当前 run 的 `ContextStore` 中读取上游输出并做 JSON 投影；
- 不直接写文件、Git 或远端系统。

已知不确定项：
- 若它消费的上游结果本身来自共享 cwd 的脆弱读取，则“只读安全”只是表面结论；
- 但从 step kind 自身语义看，仍属于低风险层。

## 4.3 `tool.sleep`：可直接并发

对应：`ToolStep{Name: "sleep"}`。

判定：**可直接并发**。

依据：
- 只在当前 run 内等待；
- 不写共享资源。

已知不确定项：
- 顶层多 run 场景中，sleep 只会拉长 wall-clock 时间，不会引入新的共享副作用。

## 4.4 `loop`：需隔离条件

对应：`steps.KindLoop`。

判定：**需隔离条件**。

依据：
- `loop` 本身是 body 的执行容器，不是独立副作用类型；
- 它支持 `parallel` 与 `max_concurrency`，会让 body 内 step 在同一 run 内并发；
- 在顶层多 formula 并发下，风险不是 loop 结构本身，而是“顶层多 run × loop body 并发”叠加后，body step 是否仍安全。

应视为安全的前提：
- body 只含 `noop` / `aggregate` / 只读 agent 或只读 script；
- 或 body 中写操作已被约束到本 run worktree / run-scoped 输出目录。

已知不确定项：
- loop 内部已经有自己的 ContextStore 克隆与 goroutine pool，但这只能隔离同一 run 的上下文，不等于隔离外部副作用。

## 4.5 `retry`：需隔离条件

对应：`steps.KindRetry`。

判定：**需隔离条件**。

依据：
- `retry` 只是 child step 的重试容器；
- 风险不来自容器，而来自 child 是否幂等、是否会重复发送消息/重复写文件/重复推远端。

应视为安全的前提：
- child step 本身是只读或明确幂等；
- 调用方能接受多次执行副作用。

已知不确定项：
- 顶层多 run 下，即使每个 run 内 retry 逻辑正确，也无法自动避免不同 run 在同一外部对象上重复操作。

## 4.6 `agent`：需隔离条件

对应：`steps.KindAgent`。

判定：**需隔离条件**。

依据：
- agent step 默认是 run-scoped 的一次推理调用；
- 但它会继承 workspace cwd，可生成文件、读当前仓库状态、输出动态 human input、触发校验失败后的修复路径；
- 若多个 run 共享 cwd，agent 的代码读取结论、文件生成、后续 patch 建议都可能互相污染。

适合纳入并发的前提：
- 主要用于只读分析、计划、报告生成；
- 若需要落盘，应写入 run-scoped 目录或 worktree；
- 不依赖共享的 final report chat / resume 交互链路。

已知不确定项：
- agent 本身是否“写文件”常常取决于提示词，而不是 kind 定义；
- 因此产品层仍需声明语义约束，不能仅凭 kind 放行。

## 4.7 `external_agent`：需隔离条件

对应：`steps.KindExternalAgent`。

判定：**需隔离条件**。

依据：
- 从 runtime 契约上，它与 `agent` 类似，也有 workspace、prompt、input_context；
- 但实际执行经由外部 CLI driver（如 `jcode` / `codex` / `opencode` / `forge`），其本地缓存、会话文件、工具调用习惯更难统一约束。

适合纳入并发的前提：
- 只做分析或内容生成；
- workspace 与输出路径受控；
- 不假设外部 agent driver 自带强隔离。

已知不确定项：
- driver 自身可能持久化 session、创建临时文件或调用更多外部工具；
- 因此整体不应比内置 `agent` 判定更乐观。

## 4.8 `script`（只读）：需隔离条件

对应：`steps.KindScript`。

判定：**需隔离条件**。

依据：
- script 可以只读，例如 `git status`、`gh pr view`、`jq`、`go test`；
- 但 runtime 只提供命令黑名单和部分危险模式检查，并不会证明该脚本“没有其他副作用”；
- 它还能通过显式 `cwd`、脚本内部 `cd`、绝对路径或外部工具缓存跳出 workspace。

适合纳入并发的前提：
- 命令明确为只读；
- 输入来源稳定；
- 若涉及仓库读取，优先在 worktree 内运行；
- 若可能触发缓存/下载/构建目录写入，需额外声明隔离目录。

已知不确定项：
- “只读 script” 经常被提示词或工具链细节破坏，例如测试框架生成缓存、语言工具链写 module cache。

## 4.9 `write_files`：建议限制

对应：`steps.KindWriteFiles` 与 `tool.write_files`。

判定：**建议限制**。

依据：
- 该 step 会真实落盘；
- 当前仅校验 `dir_name` 和 markdown 文件名的局部安全，不提供跨 run 输出目录唯一性、文件锁或冲突检测；
- 在顶层多 run 下，只要目标目录共享，就容易发生覆盖与时序竞争。

适合的限制方式：
- 要求输出路径 run-scoped；
- 或只允许写入 worktree / 临时目录；
- 聚合层需要显式标出这是写型 step。

已知不确定项：
- 即使目录 run-scoped，产出若后续又被合并回共享 docs 路径，仍可能在下游产生冲突。

## 4.10 `tool.git_*`：建议限制

对应：
- `tool.git_fetch`
- `tool.git_push`
- `tool.git_branch`
- `tool.git_checkout`
- `tool.git_worktree`

判定：**建议限制**。

依据：
- 它们比 shell `git ...` 更可见、更结构化；
- 但本质仍执行真实 Git 命令，并且没有跨 run 锁；
- `git_checkout` / `git_branch` / `git_worktree` 会修改本地工作树，`git_push` 会修改远端状态。

适合的限制方式：
- 本地仓库改动必须在 worktree 中执行；
- 对 `git_push`、共享 branch 更新、worktree remove/prune 视为高风险；
- MVP 不默认承诺多 run 下的远端 Git 安全。

已知不确定项：
- `git_fetch` 看起来较只读，但仍会更新本地 refs；
- 因此也不应和纯只读 `aggregate` / `noop` 同档。

## 4.11 写型 `script`：建议限制

对应：任何会写文件、改 Git、发外部请求、启动后台进程的 `script`。

判定：**建议限制**。

依据：
- 相比 `tool.git_*`，shell script 的可见性更低、约束更弱；
- 它可同时修改工作树、共享缓存、端口、远端系统；
- 即使在 worktree 中运行，也不能隔离仓库外副作用。

适合的限制方式：
- 第一阶段仅允许经过人工审查、语义清楚的写型脚本；
- 必须声明 workspace / 输出路径 / 外部系统影响面；
- 默认不要把它归入“安全并发”。

已知不确定项：
- 同一脚本可能既有 repo 内写入，也有远端 API 副作用，很难用单一 guardrail 覆盖。

## 4.12 `human_input`：暂不纳入 MVP

对应：`steps.KindHumanInput` 与静态 form。

判定：**暂不纳入 MVP**。

依据：
- `HumanInputStep.Run()` 会让 step 进入 `waiting`，run 状态变 `waiting_input`；
- CLI、store、dashboard 都是按单 run 去解析 waiting step、提交 input、resume；
- 在顶层多 run 聚合监视下，人工需要先判断“当前给哪个 run、哪个 waiting step 回答”，交互归属复杂。

已知不确定项：
- 系统并非不能支持多个 waiting run 共存；
- 但如果没有清晰的聚合视图与归属模型，MVP 很容易让用户误操作到错误 run。

## 4.13 动态 human input agent：暂不纳入 MVP

对应：`agent.dynamic_form = true`。

判定：**暂不纳入 MVP**。

依据：
- 它比静态 `human_input` 更复杂，因为是否提问、问什么、何时进入 waiting 都在运行时决定；
- 聚合场景下，人类操作者需要在多个 run 间理解 agent 临时生成的表单语义。

已知不确定项：
- 动态表单在单 run 流程中很有价值；
- 但在顶层并发 MVP 中，先应视为高认知负担能力。

## 4.14 自动修复 / repair / retry-step：暂不纳入 MVP

对应：
- runtime `StepFixer`
- dashboard `retry-step`
- `Repairs` 面板与 `confirm repair`

判定：**暂不纳入 MVP**。

依据：
- repair record、attempt、确认、重跑都严格绑定单个 run；
- 非幂等 step 之所以默认不自动修复，就是为了避免重复副作用；
- 顶层多 run 下，如果把 repair / retry 暴露成聚合操作，极易误把某个失败 step 当成“可安全再试”。

已知不确定项：
- 后续可以把 repair 作为高级功能引入聚合视图；
- 但第一阶段应把它保留在单 run 详情内，不作为并发能力的一部分承诺。

## 4.15 final report chat / agent session：暂不纳入 MVP

对应：
- dashboard `/api/final-report-chat`
- `/api/final-report-chat/message`
- `/api/final-report-chat/promote`
- `/api/agent-session`

判定：**暂不纳入 MVP**。

依据：
- 这些能力显式以当前 run 的最终报告、workspace、session transcript 为上下文；
- 它们天然是单 run 详情能力，而不是顶层多 run 编排能力。

已知不确定项：
- 聚合视图未来可以展示哪些 run 有 final report chat、哪些 step 有 agent transcript；
- 但不应把“统一监视”误扩展成“统一并发操作这些单 run 会话能力”。

## 5. 对 waiting / retry / repair 等 run-scoped 行为的结论

本 bead 特别需要强调：**不仅 step kind 有差异，run-scoped 行为本身也不适合被笼统地视为“并发安全”。**

### 5.1 waiting / human input

- waiting step 的识别、表单加载、用户提交、resume 都依赖单 run store；
- 在多 run 并发时，难点不是“系统能不能暂停”，而是“人如何稳定地把输入提交给正确的 run / step”。

结论：
- 在顶层多 formula 并发 MVP 中，human input 类能力应默认排除或降级为单 run 内操作。

### 5.2 retry / repair

- 自动修复依赖幂等性与单 run 上下文；
- retry-step 也依赖从失败点恢复单 run 的 snapshot 与 context。

结论：
- 顶层多 run 场景里，不应把 repair / retry 作为第一阶段默认并发能力宣传；
- 更适合作为单 run drill-down 能力保留。

### 5.3 final report chat / agent session

- 这些能力明确依附于 run 的 workspace、session transcript、final output；
- 它们不是编排层的多 run 控制面，而是单 run 后处理能力。

结论：
- 聚合层可以做状态可见性，不宜在 MVP 直接承诺统一操作体验。

## 6. 可直接给下游 beads 复用的 MVP 分层

### 6.1 第一阶段可纳入的低风险集合

- `noop`
- `aggregate`
- `tool.sleep`
- 只读 `agent`
- 只读 `external_agent`
- 只读 `script`
- body 明确低风险的 `loop`
- child 明确幂等低风险的 `retry`

但仍建议：
- repo 相关读取优先放在 worktree；
- 不共享输出路径；
- 不把 waiting / repair / final report chat 算入默认并发能力。

### 6.2 需要显式前提或用户确认的中风险集合

- `write_files`
- `tool.git_fetch` / `git_branch` / `git_checkout` / `git_worktree`
- 写型 `script`
- 可能落盘或触发副作用的 `agent` / `external_agent`

建议前提：
- 必须 worktree；
- 必须 run-scoped 输出路径；
- 必须显式声明是否会改远端或外部系统。

### 6.3 MVP 暂排除集合

- `human_input`
- 动态 human input agent
- repair / retry-step / confirm repair
- final report chat / agent session
- `git_push` 与其他会直接改共享远端状态的步骤

## 7. 已知不确定项

1. **kind 只是下限，不是完整语义**：例如 `agent` 既可以只读分析，也可以在提示词驱动下改文件；因此矩阵不能替代 formula 语义审查。
2. **只读 script 仍可能隐式写缓存**：语言工具链、测试框架、CLI 会产生隐式副作用。
3. **dashboard 与 store 仍是单 run 模型**：本 bead 只做风险分层，不等于聚合监视模型已经定义完成。
4. **repair / retry 的真正难点是操作归属，不只是执行器能力**。

## 8. 对下游 beads 的直接输入

### 对 `tt-4ce`（MVP 方向收敛）

可直接引用：
- MVP 不应承诺“所有 step 都支持顶层多 formula 并发”；
- 更现实的是分层支持：低风险只读类优先，写型与交互型显式限制。

### 对 `tt-2tr`（验证场景矩阵）

最小场景至少应覆盖：
- 两个只读 agent / script run 并发；
- worktree 下的 repo 写操作；
- 共享输出目录上的 `write_files` 冲突；
- waiting_input run 与普通 run 并发；
- repair / retry-step 在多 run 环境下的误操作边界；
- `git_push` / 外部系统写入场景为什么不纳入 MVP。

## 9. 参考模块

- `internal/formula/steps/step.go`
- `internal/formula/steps/kinds.go`
- `internal/formula/runtime/executor.go`
- `internal/formula/runtime/state.go`
- `internal/formula/ui/state.go`
- `cmd/formula/formula_dashboard_handlers.go`
- `cmd/formula/formula_human_input_test.go`
- `ai-docs/step-kinds-reference.md`
- `docs/formula/current-run-model-and-top-level-concurrency-boundaries.md`
- `docs/formula/workspace-and-side-effect-isolation-boundaries-for-multi-run.md`
