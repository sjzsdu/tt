# 多 formula 并发下的 workspace 与副作用隔离边界（tt-q3o）

本文档聚焦 bead `tt-q3o`：在 **多个 formula 作为独立 run 并发存在** 的前提下，评估共享 cwd、worktree、文件写入、git 操作、脚本执行等副作用边界，并给出适合 MVP 的隔离前提建议。

> 前置结论来自：
> - `docs/formula/multi-run-product-shape.md`（tt-f08）：第一阶段优先是“多个独立 run + 统一监视视角”；
> - `docs/formula/current-run-model-and-top-level-concurrency-boundaries.md`（tt-iqj）：当前顶层执行单位仍是单 run，loop 内并发不能等同于顶层多 formula 并发。

## 1. bead 在依赖树中的位置

- 当前 bead：`tt-q3o`，关注 **多个独立 formula run 并发存在时的 workspace / 副作用冲突**。
- 上游依赖：
  - `tt-f08` 已关闭，已收敛产品方向为“独立 run + 聚合监视”；
  - `tt-iqj` 已关闭，已确认当前 runtime 顶层仍是单 run。
- 下游阻塞：
  - `tt-2tr`：最小验证场景矩阵；
  - `tt-4ce`：MVP 方向与阶段边界；
  - `tt-ueo`：step 类型安全矩阵。

因此本 bead 只回答：**如果未来同时启动多个独立 formula run，它们在 workspace 与副作用层面会如何互相干扰，现有 worktree 能缓解哪些问题，哪些仍然需要被视为产品约束。**

## 2. 结论先行

### 2.1 默认共享 cwd 并发并不安全

若多个 formula run 都直接在同一个仓库工作目录（`env.workspace_cwd` / `env.cwd`）里执行，则当前系统中的以下能力天然会互相污染：

- `script` step 对文件系统的读写；
- `write_files` step 的落盘；
- `tool.git_*` 与 shell `git ...` 命令修改仓库状态；
- 外部 agent / 外部脚本使用相对路径产生的输出；
- 依赖当前工作树状态的检查、构建、测试和 patch 生成。

所以“能够并发启动多个 run”**不等于**“共享一个 cwd 仍然安全”。

### 2.2 现有 workflow 级 worktree 主要解决的是 Git 工作树隔离，不是全部副作用隔离

`internal/formula/runtime/workspace.go` 当前提供的 workspace 策略本质是：

- 针对单个 workflow run，按 policy 创建一个 `git worktree`；
- 通过 `SeedWorkspaceEnvironment` 把 `TT_WORKSPACE_CWD` / `env.workspace_cwd` 指向该 worktree；
- step 默认在该 workspace 里运行，除非显式覆写 `cwd`；
- 可选自动建分支、稀疏 checkout、cleanup。

这能显著降低：

- Git index / HEAD / working tree 的直接互相污染；
- 同一分支同时在多个 worktree checkout 的冲突（代码已尝试规避分支名冲突与已签出 worktree 冲突）；
- 根仓库工作目录被脚本直接改脏的概率。

但它**不能**自动解决：

- 指向仓库外部绝对路径的写入；
- 多个 run 写同一个外部输出目录；
- 脚本调用外部系统（数据库、云资源、远端 API）造成的共享副作用；
- 分支推送、远端 PR 更新、共享缓存目录修改；
- 故意把 `cwd` 改回原始仓库目录或其他共享路径的 step。

### 2.3 MVP 更适合把“隔离前提”做成产品约束，而不是假设 runtime 已经彻底防护

对第一阶段 MVP，更现实的前提是：

1. **默认仅纳入只读或弱副作用 formula**；
2. 需要写 Git 工作树时，**要求启用 worktree workspace**；
3. 不允许多个 run 共享同一显式输出路径；
4. 对会推送远端、改共享分支、发外部请求的 formula，先视为高风险场景；
5. 聚合层先做“可见性与约束声明”，不要把“并发安全”错误包装成已解决。

## 3. 当前 workspace 运行机制：系统真实保证了什么

## 3.1 workspace policy 是 workflow 级，不是 step 级沙箱

`Executor.Run()` 在真正执行步骤前，会调用 `prepareWorkspace()`：

1. 读取 `workflow.Workspace` policy；
2. 当前仅支持 `kind = "worktree"`；
3. 要求当前环境是 git repo；
4. 基于 invocation cwd 解析工作区路径；
5. 必要时在 `.tt/worktrees/<run-id>` 下创建 worktree；
6. 可选设置 sparse-checkout；
7. 可选创建/切换 branch；
8. 把环境变量中的 workspace 路径改为该 worktree。

这说明现有隔离单位是：

- **一个 workflow run 一个 workspace session**；
- 不是“每个 step 一个临时沙箱”；
- 也不是“同一个 run 里副作用自动回滚”。

因此，如果一个 run 内先后多个 step 改坏了 workspace，后续 step 会继续看到被改坏后的状态；多 run 并发只是把这个风险从“单 run 内污染”扩展成“多个 run 之间是否共享污染源”。

## 3.2 step 默认继承 workspace cwd，但允许显式逃逸

`steps.renderStepCwd()` 的解析逻辑是：

1. 若 step 显式配置了 `cwd`，优先使用渲染后的该路径；
2. 否则回退到 `env.cwd`（运行时会把它 seed 为 workspace cwd）。

含义：

- 在启用 workspace policy 时，很多 step 默认会落到 worktree 内执行；
- 但只要 formula 作者显式把 `cwd` 指向原仓库目录、父目录、或其他共享目录，隔离就会被绕开；
- 当前 runtime **没有**统一禁止这类 escape。

所以 worktree 更像“默认安全路径”，不是强制沙箱边界。

## 4. 主要冲突类型

## 4.1 文件覆盖与内容竞争

最直接的风险是多个 run 共享同一文件路径：

- 多个 `script` step 用相对路径写同一个文件；
- 多个 `write_files` step 把文档写到同一目录下的同名文件；
- agent step 在共享 cwd 里生成、覆盖或删除同一批文件；
- 一个 run 读取时，另一个 run 正在改写该文件。

`write_files` step 只做了：

- `dir_name` 必须是单段安全路径；
- 文件名必须是安全的 `.md` 文件名。

它**没有**做：

- 跨 run 的唯一目录约束；
- “目标文件已存在是否来自别的 run”的判断；
- 原子写 / 锁 / 乐观并发控制。

因此：

- `write_files` 对单 run 文档落盘很方便；
- 但在顶层多 run 并发下，如果目标目录是共享的，就会出现覆盖风险。

## 4.2 Git 状态污染

共享仓库目录下执行的 Git 相关步骤会互相影响：

- `git checkout` / `git branch` / `git worktree remove` / `git push` 会改变仓库状态；
- shell `script` 中的 `git add/commit/rebase/merge/reset` 也会直接改工作树和引用；
- `run.Store` 记录的 branch / commit / dirty 状态可能在运行中被其他 run 改变，导致元数据与实际执行时刻不一致。

现有 `tool.git_*` 实现只是把命令封装成 deterministic step，本身仍然通过 `exec.CommandContext` 执行真实 Git 命令；并没有提供跨 run 锁。

## 4.3 branch / worktree 冲突

当前 `workspace.go` 已考虑两类局部冲突：

- 本地 ref 路径冲突（如 `feature/x` 与已有 `feature` branch 名冲突）；
- 某 branch 已被别的 worktree checkout，于是自动加 run-id 后缀避开。

这说明系统已经承认：**同一 repo 中多个 worktree 并发存在时，branch 名和 checkout 状态本来就会冲突。**

但仍有未解决风险：

- 多个 run 即使在不同 worktree，也可能都要推送同一个远端 branch；
- 一个 run 删除 worktree / prune 时，另一个 run 的引用关系可能变化；
- 公式若显式调用 `git worktree remove` 指向共享路径，仍可破坏其他 run。

## 4.4 外部脚本副作用

`script` step 与 `tool.git_*` 最关键的风险不是“会不会执行”，而是“执行结果是否局限在当前 workspace 内”。

典型风险：

- 脚本写入 `/tmp/...`、用户 home、共享 cache、仓库外构建目录；
- 调用 `docker`, `npm`, `pip`, `go` 等工具时读写全局缓存；
- 更新数据库、消息队列、远端工单、PR comment、issue、聊天消息；
- 启动长生命周期后台进程，占用相同端口或锁文件。

当前 runtime 会把 `TT_INVOCATION_CWD`、`TT_WORKSPACE_CWD`、`TT_FORMULA_RUN_DIR` 注入脚本环境，这有助于脚本“自觉”使用隔离路径；但系统并**不会强制**脚本只在这些目录内活动。

## 4.5 读写时序问题

即使多个 run 都只是“读多写少”，仍可能有时序耦合：

- 一个 run 期望仓库未变更，另一个 run 正在改 branch 或生成文件；
- 一个 run 的测试 / lint 结果依赖仓库瞬时状态；
- 一个 run 在读取 agent transcript、snapshot、生成 diff 时，另一个 run 改变了同一份工作树。

所以“只读步骤安全”只在 **输入源也只读且稳定** 时才成立。

## 5. 现有 worktree 策略能解决什么，不能解决什么

## 5.1 能解决或显著缓解的部分

### A. 隔离 Git working tree

每个 run 用独立 worktree 时：

- 未提交文件不会直接落在同一个工作目录里；
- `git status` / `git add` / `git commit` 的对象主要局限在各自 worktree；
- 许多基于相对路径的脚本会自然落在本 run 的 workspace 下。

### B. 给每个 run 一个相对稳定的 cwd

通过 `env.workspace_cwd` + `TT_WORKSPACE_CWD`：

- script / agent / external_agent 默认更容易在独立目录执行；
- dashboard 也能展示当前 run 的 workspace 路径；
- 后续产品可据此标记“该 run 是否使用隔离 workspace”。

### C. 对 branch checkout 冲突做了局部规避

已有逻辑会在必要时：

- 修正 branch 名避免本地 ref path 冲突；
- 检测 branch 已在别的 worktree checkout 时自动派生候选名。

这对“多个 run 在同一 repo 中并发开工作区”是很实用的基础能力。

## 5.2 不能解决的部分

### A. 不能阻止 step 主动跳出 workspace

显式 `cwd`、绝对路径、脚本内部 `cd` 都可以离开 worktree。

### B. 不能隔离仓库外部副作用

比如：

- 网络请求；
- 外部 API / PR / issue 修改；
- 全局缓存、端口、锁、临时目录；
- 共享数据库 / 队列。

### C. 不能提供跨 run 文件锁与输出路径仲裁

当前没有：

- 某目录被哪个 run 占用的 registry；
- 对共享输出目录的互斥保护；
- 写文件冲突检测。

### D. 不能自动保证远端 Git 操作安全

不同 worktree 仍可能：

- 推同一个 branch；
- force push 覆盖对方；
- 基于过期 base 做 rebase / merge。

## 6. 对 MVP 的隔离前提建议

## 6.1 推荐纳入 MVP 的低风险前提

适合第一阶段并发纳入的场景：

1. **纯观察 / 纯分析 / 只读型 formula**
   - 例如读仓库、生成报告、提取元数据、列历史 runs；
   - 不写共享目录，不推远端，不改外部系统。

2. **要求 worktree 的 repo 内写操作**
   - 允许写工作树、生成 patch、跑测试；
   - 但必须在独立 worktree 内完成；
   - 且默认不共享输出路径。

3. **输出路径可参数化并按 run 隔离**
   - 例如输出目录包含 run id / formula slug / batch slug；
   - 避免多个 run 写同一 `docs/foo`。

## 6.2 推荐作为产品约束显式声明的高风险场景

第一阶段应默认排除或显式警告：

1. **共享 cwd 无 worktree 的写操作 formula**；
2. **会 push / rebase / merge / checkout 共享分支的 formula**；
3. **会写仓库外绝对路径或共享缓存目录的 script**；
4. **会调用外部可变系统（PR、ticket、chat、DB、cloud）且缺少幂等/去重控制的 formula**；
5. **固定输出路径不可参数化的 write_files / script 产物写入**。

## 6.3 对 MVP 最实用的规则集

可以把第一阶段规则收敛为：

- **只读 formula**：可共享同一 repo，但仍建议独立 run；
- **会改 repo 的 formula**：必须启用 worktree；
- **会写文件的 formula**：必须保证输出路径 run-scoped；
- **会推远端或改外部系统的 formula**：先不承诺并发安全，默认人工串行或显式 opt-in；
- **聚合监视层**：必须显示每个 run 是否使用 worktree、workspace 路径、是否存在高风险副作用 step。

## 7. 哪些风险属于产品约束，哪些属于后续实现问题

## 7.1 更适合作为产品约束的

这些问题不应在 MVP 被伪装成“已经被 runtime 自动解决”：

- 某 formula 是否允许共享 cwd 并发；
- 某 formula 是否允许推远端 / 写外部系统；
- 输出路径是否必须唯一；
- 哪类 formula 只能串行运行；
- 聚合视图是否只做观察，不提供批量 stop / repair / input。

原因是：这些本质上依赖 formula 语义，而不只是依赖执行器技术细节。

## 7.2 更适合作为后续实现问题的

这些可以留给后续 beads / 阶段增强：

- 基于 step 类型或 policy 自动判断风险等级；
- 对显式 `cwd` escape 做静态/运行时告警；
- 为写文件和 Git 操作增加更强的 guardrail；
- 增加 run group 级别的 workspace / side effect 元数据；
- 构建更严格的沙箱、锁、租约或输出目录 registry。

## 8. 对下游 beads 的直接输入

### 对 `tt-4ce`（MVP 收敛）

可直接继承：

- MVP 不应默认宣称“任意 formula 都可安全并发”；
- 更合适的是“只读优先、worktree 必需、共享输出路径禁止、外部副作用显式警告”。

### 对 `tt-ueo`（step 安全矩阵）

可直接继承：

- `script`、`tool.git_*`、`write_files`、agent/external_agent 的风险差异要单独建矩阵；
- 是否使用 worktree、是否显式 cwd、是否写外部系统，应成为矩阵维度。

### 对 `tt-2tr`（验证场景矩阵）

可直接继承：

- 最小验证场景至少应覆盖：
  - 共享 cwd 文件覆盖；
  - worktree 分支冲突规避；
  - 共享输出目录冲突；
  - 远端 branch / 外部系统副作用不可隔离场景。

## 9. 参考模块

- `internal/formula/runtime/workspace.go`
- `internal/formula/runtime/environment.go`
- `internal/formula/runtime/executor.go`
- `internal/formula/steps/kinds.go`
- `internal/formula/runtime/capabilities.go`
- `internal/formula/builtin/formulas/**/*.toml`
- `ai-docs/overview.md`
- `ai-docs/builtin-formulas.md`
- `docs/formula/multi-run-product-shape.md`
- `docs/formula/current-run-model-and-top-level-concurrency-boundaries.md`
