# 内置 Formula 与 Atomic 清单

> 最后更新：2026-06-08

`tt` 仓库自带一组 `tt formula run` 即可直接使用的 workflow 模板（`formulas/`）和可被 inline include / `embed` / `expand` 复用的原子步骤（`atomics/`）。本目录列出现在仓库里实际打包的内容，帮助：

- 选一个最接近的 builtin 改写或 fork，作为新公式起点
- 知道每个 atomic 的边界能力，避免在公式中重复实现
- 知道哪些 preflight 会失败，提前准备好依赖 CLI

源代码位置：

- `internal/formula/builtin/formulas/*.toml`（12 个）
- `internal/formula/builtin/atomics/*.toml`（12 个）

加载与展示：

- 嵌入式加载：`internal/formula/builtin/load.go` 把上述目录通过 `embed.FS` 注入；用户 formula 在 `~/.config/tt/formulas/`（与工作目录 `.tt/formulas/`）里可覆盖 / 扩展。
- 列表：`tt formula list --builtin`、`tt formula list --user`、`tt formula list --category github`。
- 查看：`tt formula show <name>` / `tt formula show <name> --markdown`。
- 复制为本地工作副本：`tt formula copy <name> [output.toml]`。

## Formulas（12 个）

按 `category` 分组，描述均来自对应 toml 的 `description` 字段。

### github

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `github-pr-review` | 对单个 PR 完成代码审阅并默认真实提交 review | `[preflight]` 强校验 `gh` 命令与 `gh auth status`；组合 `github-fetch-pr` / `github-fetch-pr-files` / `github-fetch-pr-diff` / `github-build-pr-context` 等 atomic；末尾用确定性脚本提交 review 与 comment |
| `github-pr-rebase-main` | 把 PR 分支 rebase 到最新 base/main | 隔离 worktree + agent 处理冲突 + `git push --force-with-lease` 推送；默认 `cleanup` 策略处理临时 worktree |
| `github-pr-fix-comments` | 遍历未解决的 review comments 逐条用 `bug-fix` 修复 | 复用 `bug-fix` workflow（嵌套 typed runtime） |
| `github-my-prs-rebase-main` | 批量 rebase 当前用户所有 open PR | 先用 `github-list-my-prs` 拉列表，再并发复用 `github-pr-rebase-main` |
| `code-docs` | 解读代码文件、目录或 GitHub 仓库并生成 Markdown 文档 | 抽取代码证据，按规模动态规划多篇文档，并生成 README 摘要 |

### workflow / 决策

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `san-sheng-liu-bu` | 三省六部工作流：起草 → 审核 → 拆任务 → 并发生成 → 汇总 | `zhongshu-draft` / `menxia-review` loop / `shangshu-decompose` 拆 `tasks` 数组 / `foreach parallel` 生成各部启动文稿 / `aggregate` 汇总 |
| `shan-yi-zhe` | 善易者风格的多层动态决策与建议 | 必要时 typed runtime 显式澄清，展示卦序、卦象、上下卦、卦辞、彖传、爻辞，最终落到现实行动建议 |
| `feature` | 通用代码 feature 端到端实现：需求理解/动态澄清 → code-context 调研 → 计划 → 编码 → 测试方式 → 影响评估 → 报告 | 不自动 commit；适合作为默认 feature 开发入口，`validation_command` 可覆盖自动验证命令 |
| `gongbu` | 代码 feature 的端到端实现：理解需求 → 调研 → 方案 → 实现 → 验证 → 按需提交 | 使用 `[workspace] kind = "worktree"` 创建隔离分支；按需 push |
| `bug-fix` | 调试 / 修复 bug，并在结论中说明"问题不成立"的备选路径 | 第一步是动态澄清（`form = true`），输出 strict compact JSON；后续 step 串接代码调研、定位、修复、验证 |

### docs / 学习

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `fresh-topic-docs` | 针对"新鲜事物 / 概念 / 趋势"主题，澄清范围后用 foreach loop 并发生成系列 Markdown 文档 | 动态表单 + foreach 数组 + 并发 agent 写作 + aggregate 汇总 |
| `code-docs` | 见上 docs 表 |  |

### jira

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `jira-bug-fix` | 从 Jira ticket 直接拉数据并嵌套 `bug-fix` | `[preflight]` 校验 `jira` CLI；第一步用 `script` step 调 `jira issue view --comments 20 --raw` 取结构化数据 |

## Atomics（12 个）

Atomic 是 `type = "atomic"` 的最小可复用工作单元，可被公式用 `[[steps]] id = "..." execution = "agent" input_context = [...]` 模式 inline 引用，也可在 `compose` / `embed` / `expand` 段被展开。

### git

| Atomic | 输出 | 关键行为 |
| --- | --- | --- |
| `git-status-check` | 结构化 `git status` 信息 | `vars.require_clean` / `vars.require_upstream`；输出 `ok` / `branch` / `head` / `upstream` / `dirty` / `conflict` / `rebase` / `merge` / `cherry-pick` |
| `git-run-validation` | 跑任意验证命令并产出结构化结果 | `vars.command` 为空时跳过并返回 success；适合"不指定命令就跑测试" |
| `git-auto-detect-validation` | 按仓库标记自动挑选验证命令 | `python3 -c` 检测 `package.json`/`go.mod`/`pyproject.toml`/`Cargo.toml`/`Makefile` 等；`vars.command` 可覆盖 |
| `git-push-branch` | push HEAD 到指定 remote/branch | 默认 `no_verify=true` / `force_with_lease=true`；**远端副作用，需由上层公式显式组合** |
| `git-integrate-ref` | 把当前分支集成目标 ref | 脚本负责确定性 git 操作，agent 处理冲突 / 验证失败等不可编程环节；**不包含 push** |

### github

| Atomic | 输出 | 关键行为 |
| --- | --- | --- |
| `github-list-my-prs` | 当前仓库指定作者 open PR 列表 | 依赖 `gh` + `gh auth`；默认 `author = @me`；输出 `items` 数组 |
| `github-fetch-pr` | 单个 PR 的结构化元数据 | 接受编号 / URL / 分支引用；`repo_hint` 可覆盖上下文 |
| `github-fetch-pr-files` | PR 改动文件列表 | 配合 `github-fetch-pr` 使用 |
| `github-fetch-pr-diff` | PR patch diff，包装为 JSON | 适合在 review / 上下文构建时直接使用 |
| `github-build-pr-context` | 把 metadata + files + diff 摘要规整为审阅上下文 | `vars.meta_json` / `vars.files_json` / `vars.patch_chars` / `vars.review_focus` / `vars.submit_review` |

### repo

| Atomic | 输出 | 关键行为 |
| --- | --- | --- |
| `repo-prepare-local-or-clone` | 把仓库（本地目录 / GitHub URL / owner-repo）准备好 | 必要时浅克隆到 `.tt/tmp/repos`；输出 `repo_path` / `source` 等基础信息 |
| `repo-evidence-map` | 仓库轻量证据地图 | 扫描 README、配置、入口、测试和主要源码摘要；`vars.focus` 可指定关注点 |

## preflight 依赖一览

把全部 builtin 用到的 preflight 命令汇总到一张表，方便一次性准备：

| 命令 / 状态 | 出现位置 |
| --- | --- |
| `gh`（command） | `github-pr-review` / `github-pr-rebase-main` / `github-pr-fix-comments` / `github-my-prs-rebase-main` / `github-list-my-prs` / `github-fetch-pr` / `github-fetch-pr-files` / `github-fetch-pr-diff` |
| `gh auth status`（exec） | 同上 github 系列 |
| `git`（command） | `github-pr-rebase-main` / `github-pr-fix-comments` / `github-my-prs-rebase-main` / `code-docs` / `git-status-check` / `git-push-branch` / `git-integrate-ref` / `git-run-validation` / `git-auto-detect-validation` / `repo-prepare-local-or-clone` |
| `git` + `require_repo = true` | `github-pr-review` / `gongbu` / `git-status-check` / `git-push-branch` / `git-integrate-ref` / `git-run-validation` / `git-auto-detect-validation` |
| `git` + `require_remote = true` | `git-push-branch` |
| `jq`（command） | `git-status-check` / `git-run-validation` / `git-push-branch` / `git-integrate-ref` / `github-list-my-prs` / `github-fetch-pr` 等 |
| `python3`（command） | `code-docs` / `gongbu` / `git-auto-detect-validation` / `repo-prepare-local-or-clone` / `repo-evidence-map` |
| `bash`（command） | `git-run-validation` / `git-integrate-ref` |
| `jira`（command） | `jira-bug-fix` |

如果 `tt formula run <name>` 报"preflight 失败"，直接对照这张表确认是否需要安装 / 登录。

## 推荐起点

- 写"批处理 + 改写 + commit"类工作流：从 `github-pr-rebase-main` 拷贝。
- 写"动态澄清 + 调研 + 决策"：从 `bug-fix` 拷贝。
- 写"知识整理 / 文档生成"：从 `fresh-topic-docs` 拷贝。
- 写"代码 feature 端到端"：默认从 `feature` 拷贝；如果需要隔离 worktree 与按需提交，再参考 `gongbu`。
- 写"PR / Jira 自动化"：从 `github-pr-review` / `jira-bug-fix` 拷贝，然后嵌入对应的 atomic 子步骤。

## 用户公式优先级

- 用户目录中的 `.toml` 优先于 builtin 同名公式（[推测]）—— 这与 `internal/formula/builtin/load.go` 把 builtin 注入到下层 namespace 的常见做法一致；同名时本地覆盖，便于就地修改而无需升级 `tt`。
- 想临时回到 builtin：`tt formula run --builtin <name>` 形式未在当前 CLI 中暴露（[推测]）；如需使用，复制到本地再 `tt formula copy`。

## 自我修复与 builtin

builtin / atomic 中的 `script` step 默认 `idempotent = false`，runtime 不会自动重试。要让某个 command（如 `gh pr view`、`git status`、`jq …` 这类只读命令）参与 `StepFixer` 自我修复，必须在 fork 后显式写 `idempotent = true`。详见 [Step Kinds 参考 · 通用字段](./step-kinds-reference.md#通用字段) 与 [Formula 系统 · 自我修复](./formula-system.md#自我修复self-repair)。
