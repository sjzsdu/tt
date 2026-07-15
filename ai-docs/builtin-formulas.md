# 内置 Formula 与 Atomic 清单

> 最后更新：2026-06-12

`tt` 仓库自带一组 `tt formula run` 即可直接使用的 workflow 模板（`formulas/`）和可被 inline include / `embed` / `expand` 复用的原子步骤（`atomics/`）。本目录列出现在仓库里实际打包的内容，帮助：

- 选一个最接近的 builtin 改写或 fork，作为新公式起点
- 知道每个 atomic 的边界能力，避免在公式中重复实现
- 知道哪些 preflight 会失败，提前准备好依赖 CLI

源代码位置：

- `internal/formula/builtin/formulas/<category>/*.toml`（15 个）
- `internal/formula/builtin/atomics/<category>/*.toml`（3 个）

加载与展示：

- 嵌入式加载：`internal/formula/builtin.go` 把上述目录通过 `embed.FS` 注入；用户 formula 在 `~/.config/tt/formulas/`（与工作目录 `.tt/formulas/`）里可覆盖 / 扩展。
- 列表：`tt formula list --builtin`、`tt formula list --user`、`tt formula list --category github`。
- 查看：`tt formula show <name>` / `tt formula show <name> --markdown`。
- 复制为本地工作副本：`tt formula copy <name> [output.toml]`。

## Formulas（31 个）

按 `category` 分组，描述均来自对应 toml 的 `description` 字段。

### github

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `github-pr-review` | 对单个 PR 完成代码审阅并默认真实提交 review | `[preflight]` 强校验 `gh` 命令与 `gh auth status`；组合单个 `github-fetch-pr` atomic 获取 PR 全量信息；末尾用确定性脚本提交 review 与 comment |
| `github-pr-rebase-main` | 把 PR 分支 rebase 到最新 base/main | 隔离 worktree + agent 处理冲突 + `git push --force-with-lease` 推送；默认 `cleanup` 策略处理临时 worktree |
| `github-pr-fix-comments` | 遍历未解决的 review comments 逐条用 `bug-fix` 修复 | 复用 `bug-fix` workflow（嵌套 typed runtime） |
| `github-my-prs-rebase-main` | 批量 rebase 当前用户所有 open PR | 先用 `github-list-my-prs` 拉列表，再并发复用 `github-pr-rebase-main` |
| `github-my-prs-fix-comments` | 批量处理当前用户所有 open PR 的未解决 review comments | 先用 `github-list-my-prs` 拉列表，再并发复用 `github-pr-fix-comments` |

### workflow / 决策

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `san-sheng-liu-bu` | 三省六部工作流：起草 → 审核 → 拆任务 → 并发生成 → 汇总 | `zhongshu-draft` / `menxia-review` loop / `shangshu-decompose` 拆 `tasks` 数组 / `foreach parallel` 生成各部启动文稿 / `aggregate` 汇总 |
| `shan-yi-zhe` | 善易者风格的人生参谋、决策建议与历史情境易理复盘 | 先辨事并确定“此人此事此时此位”，识别 personal_advice / historical_case_study；必要时 typed runtime 显式澄清，再取卦定像，解释本卦、卦辞/彖传、关键爻与变卦；个人问题落到现实行动，历史问题复盘人物当时之局、选择、结果与今人可学的易学智慧 |
| `feature` | 通用代码 feature 端到端实现：需求理解/动态澄清 → code-context 调研 → 计划 → 编码 → 测试方式 → 影响评估 → 报告 | 不自动 commit；适合作为默认 feature 开发入口，`validation_command` 可覆盖自动验证命令 |
| `gongbu` | 代码 feature 的端到端实现：理解需求 → 调研 → 方案 → 实现 → 验证 → 按需提交 | 使用 `[workspace] kind = "worktree"` 创建隔离分支；按需 push |
| `bug-fix` | 调试 / 修复 bug，并在结论中说明"问题不成立"的备选路径 | 第一步是动态澄清（`form = true`），输出 strict compact JSON；后续 step 串接代码调研、定位、修复、验证 |
| `requirement-grooming` | 需求整理器：根据用户的模糊需求调研项目，并用 `bead-manager` 创建或规划一组可执行 beads | 动态澄清需求 → code-research 调研项目 → 设计候选 backlog 和 dependency_edges → bead-manager 去重、创建/更新 beads 并用 `bd link <blocked> <blocker>` 建立依赖；支持 `create_mode=plan` 只出计划，最终输出 Markdown 报告 |
| `git-resolve-merge-conflicts` | 只处理当前 git 项目中已经存在的 merge/rebase/cherry-pick 冲突 | 为每个冲突文件生成上下文，并发调用 resolver agent 修复，必要时重建 lockfile，最后运行可选验证 |

### coding

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `coding` | 统一编码入口：需求计划 → 编码实现 → 总报告 | 以 FormulaCall 编排 `coding-requirement` 和 `coding-implementation`，父子层只通过公开 inputs/outputs 交互；保持原 `coding` / `coder` 入口可用；全程无动态表单 |
| `coding-requirement` | 单独完成无交互需求理解、计划和需求评审 | 输出可执行计划、验收标准、影响范围、风险和测试计划；信息不足时用 assumptions 继续推进 |
| `coding-implementation` | 基于已收敛需求计划完成实现、代码评审和验证 | 执行实现、代码评审循环和 `run-validation`；不负责 commit |
| `bead-coding` | 执行单个 ready bead 树：读取 bead 详情和依赖，通过 FormulaCall 调用无交互 `coding` 完成实现与验证，再提交并按需关闭 bead | 公开 `cycle_summary` 与 Markdown `report`；自动选择 ready bead 或使用指定 `bead_id`；提交成功后可 `bd close` |
| `keep-coding` | 持续处理 ready beads：runtime loop 每轮以 FormulaCall 调用 `bead-coding`，完成一个 bead 树的实现、验证、提交和关闭 | 循环只读取公开 `cycle_summary`，不依赖 bead-coding 内部 step；通过 `max_cycles` 控制轮数 |
| `requirement-discovery` | 需求发现与澄清入口 | 面向 coding 目录的需求探索辅助 formula |

### docs / 学习

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `fresh-topic-docs` | 针对"新鲜事物 / 概念 / 趋势"主题，澄清范围后用 foreach loop 并发生成系列 Markdown 文档 | 动态表单 + foreach 数组 + 并发 agent 写作 + aggregate 汇总 |
| `code-docs` | 解读代码文件、目录或 GitHub 仓库并生成 Markdown 文档 | 抽取代码证据，按规模动态规划多篇文档，并生成 README 摘要 |
| `code-arch` | 为代码文件、目录或 GitHub 仓库生成架构文档包 | 新增 `arch-map.py` 抽取入口、模块聚类、依赖边、API/状态/数据/风险线索；规划并发撰写包含 D2 图块的架构 Markdown |

### jira

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `jira-bug-fix` / `jira-feature` | 从 Jira ticket 拉取数据并以 FormulaCall 调用 `bug-fix` / `feature` | `[preflight]` 校验 `jira` CLI；父 Formula 只消费子 Formula 的公开 `report`，不读取内部 step |

### examples

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `external-agent-review` | 演示如何把当前 git diff 交给外部 agent CLI 审查 | 支持 jcode/codex/opencode/forge/bl 等外部命令，并用内置 reporter 汇总 |

## Atomics（3 个）

Atomic 是当前 builtin formula 通过 `embed = "..."` 复用的最小原子步骤。未被 builtin formula 使用的 atomic 不保留，避免目录膨胀。

| Atomic | 被谁使用 | 输出 | 关键行为 |
| --- | --- | --- | --- |
| `run-validation` | `feature` / `gongbu` / `github-pr-rebase-main` | `validation.stdout` | 显式运行 `vars.command`；`auto_detect=true` 且 command 为空时根据 `go.mod` / `package.json` / `pnpm-lock.yaml` / `pyproject.toml` 自动选择轻量验证 |
| `github-fetch-pr` | `github-pr-review` / `github-pr-fix-comments` / `github-pr-rebase-main` | `pr.stdout` | 一次获取单个 PR 的 metadata、files、patch diff 和 review 上下文字段；父 formula 自行取需要字段 |
| `github-list-my-prs` | `github-my-prs-fix-comments` / `github-my-prs-rebase-main` | `prs.stdout` | 列出当前仓库指定作者的 open PR，供批量流程循环 |

## preflight 依赖一览

把全部 builtin 用到的 preflight 命令汇总到一张表，方便一次性准备：

| 命令 / 状态 | 出现位置 |
| --- | --- |
| `gh auth status`（exec） | 同上 github 系列 |
| `git`（command） | `github-pr-rebase-main` / `github-pr-fix-comments` / `github-my-prs-rebase-main` / `code-docs` / `run-validation` / `bead-coding` / `keep-coding` |
| `git` + `require_repo = true` | `github-pr-review` / `gongbu` / `run-validation` / `bead-coding` / `keep-coding` |
| `jq`（command） | `run-validation` / `github-list-my-prs` / `github-fetch-pr` 等 |
| `python3`（command） | `code-docs` / `gongbu` |
| `bash`（command） | `run-validation` |
| `bd`（command） | `requirement-grooming` / `bead-coding` / `keep-coding` |
| `jira`（command） | `jira-bug-fix` |

如果 `tt formula run <name>` 报"preflight 失败"，直接对照这张表确认是否需要安装 / 登录。

## 推荐起点

- 写"批处理 + 改写 + commit"类工作流：从 `github-pr-rebase-main` 拷贝。
- 写"动态澄清 + 调研 + 决策"：从 `bug-fix` 拷贝。
- 写"需求整理 + 项目调研 + beads backlog"：从 `requirement-grooming` 拷贝。
- 写"beads 驱动的持续编码"：用 `keep-coding`；只想执行单个 ready bead 时用 `bead-coding`。
- 写"知识整理 / 文档生成"：从 `fresh-topic-docs` 拷贝。
- 写"代码 feature 端到端"：默认从 `feature` 拷贝；如果需要隔离 worktree 与按需提交，再参考 `gongbu`。
- 写"PR / Jira 自动化"：从 `github-pr-review` / `jira-bug-fix` 拷贝，然后嵌入对应的 atomic 子步骤。

## 与多 formula / 多 run 研究最相关的现有模式

这部分只盘点**最接近后续 `tt-2tr` / `tt-4ce` 的真实拼图**，帮助后续 bead 先复用现有能力，而不是把需求描述成从零设计。

| 模式 | 现有样例 / 入口 | 可直接复用 | 不能覆盖什么 |
| --- | --- | --- | --- |
| loop 并发 / foreach 并发 | `san-sheng-liu-bu`、`fresh-topic-docs`、`keep-coding` | 证明 runtime 已支持单个 run 内的 foreach/loop 并发、分批汇总与循环推进；很适合拿来构造“一个 run 里并发处理多任务”的对照样例 | 不能代表多个顶层 formula run 的统一调度、统一取消、统一状态页 |
| schedule 触发 | `tt formula schedule`（CLI 与 tests 在 `cmd/formula/formula_schedule_test.go`） | 可直接复用 every / cron / run-now / max-runs 这些已有触发语义，作为未来多 run 场景的“启动条件” | schedule 只负责决定何时启动新 run，不提供跨 run 聚合观察或批次管理 |
| worktree 隔离工作区 | `gongbu`、`github-pr-rebase-main` | 可直接复用 run 级隔离 workspace、自动 cleanup、agent/script 在独立目录中执行的约束 | 不能替代多个 run 之间的资源池、批量 worktree 总览或跨 run 资源编排 |
| resume / human input | `bug-fix` 的动态澄清入口；runtime/CLI 对 human input 与 resume 的支持 | 可复用等待态、人类补充输入后继续执行、失败后 resume 等单 run 生命周期能力 | 仍围绕单个 run snapshot 设计，不能直接回答“多个 run 谁在等人、如何统一提交输入” |
| 嵌套 formula / 复用公式 | `coding` → `coding-requirement` / `coding-implementation`、`keep-coding` → `bead-coding` | 证明 FormulaCall 能在普通节点和 runtime loop 中通过公开 map 契约递归编排，并保留独立层级状态 | 每个 FormulaCall 仍属于同一个顶层 run，不是跨 run 调度平面 |

### 建议优先深挖的测试 / 样例入口

- `cmd/formula/formula_runtime_test.go`
  - workspace guard 注入
  - loop iteration session 隔离
  - dashboard snapshot 映射
  - resume dependency exclusions（尤其 loop ancestor 重跑）
- `cmd/formula/formula_schedule_test.go`
  - every / cron / max-runs 的核心行为
- `cmd/formula/formula_human_input_test.go`
  - human input CLI 路径与字段提交
- `cmd/formula/formula_dashboard_updates_test.go`
  - timeline、workspace ready、final report chat 等 reopen/dashboard 相关路径
- `internal/formula/runtime/formularun_store_test.go`
  - snapshot / run store / waiting-human-input 状态落盘
- `internal/formula/runtime/executor_test.go`
  - runtime 执行、repair/fixer、workspace/environment 等底层夹具

如果后续目的是定义“多 run 最小场景矩阵”，优先从这些现有样例反推覆盖面，再补缺口；不要先抽象再找例子。

## 目录组织原则

- `formulas/github/`: GitHub PR 批处理、审阅、rebase、评论修复。
- `formulas/engineering/`: 通用代码实现、修 bug、需求整理、单 bead 编码、冲突解决、Jira bug 修复。
- `formulas/docs/`: 代码或主题文档生成。
- `formulas/workflow/`: 方法论 / 决策型流程，以及跨 bead 的持续编码 loop。
- `formulas/examples/`: 示例或集成演示。
- `atomics/github/`, `atomics/validation/`: 可被父 formula 复用的最小原子步骤。

路径只影响维护组织，运行时仍按 TOML 中的 `formula = "..."` 名称加载。

## 用户公式优先级

- 用户目录中的 `.toml` 优先于 builtin 同名公式（[推测]）—— 这与 `internal/formula/builtin.go` 把 builtin 注入到下层 namespace 的常见做法一致；同名时本地覆盖，便于就地修改而无需升级 `tt`。
- 想临时回到 builtin：`tt formula run --builtin <name>` 形式未在当前 CLI 中暴露（[推测]）；如需使用，复制到本地再 `tt formula copy`。

## 自我修复与 builtin

builtin / atomic 中的 `script` step 默认 `idempotent = false`，runtime 不会自动重试。要让某个 command（如 `gh pr view`、`git status`、`jq …` 这类只读命令）参与 `StepFixer` 自我修复，必须在 fork 后显式写 `idempotent = true`。详见 [Step Kinds 参考 · 通用字段](./step-kinds-reference.md#通用字段) 与 [Formula 系统 · 自我修复](./formula-system.md#自我修复self-repair)。
