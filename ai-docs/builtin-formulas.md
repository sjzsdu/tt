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

## Formulas（15 个）

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
| `shan-yi-zhe` | 善易者风格的多层动态决策与建议 | 必要时 typed runtime 显式澄清，展示卦序、卦象、上下卦、卦辞、彖传、爻辞，最终落到现实行动建议 |
| `feature` | 通用代码 feature 端到端实现：需求理解/动态澄清 → code-context 调研 → 计划 → 编码 → 测试方式 → 影响评估 → 报告 | 不自动 commit；适合作为默认 feature 开发入口，`validation_command` 可覆盖自动验证命令 |
| `gongbu` | 代码 feature 的端到端实现：理解需求 → 调研 → 方案 → 实现 → 验证 → 按需提交 | 使用 `[workspace] kind = "worktree"` 创建隔离分支；按需 push |
| `bug-fix` | 调试 / 修复 bug，并在结论中说明"问题不成立"的备选路径 | 第一步是动态澄清（`form = true`），输出 strict compact JSON；后续 step 串接代码调研、定位、修复、验证 |
| `git-resolve-merge-conflicts` | 只处理当前 git 项目中已经存在的 merge/rebase/cherry-pick 冲突 | 为每个冲突文件生成上下文，并发调用 resolver agent 修复，必要时重建 lockfile，最后运行可选验证 |

### docs / 学习

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `fresh-topic-docs` | 针对"新鲜事物 / 概念 / 趋势"主题，澄清范围后用 foreach loop 并发生成系列 Markdown 文档 | 动态表单 + foreach 数组 + 并发 agent 写作 + aggregate 汇总 |
| `code-docs` | 解读代码文件、目录或 GitHub 仓库并生成 Markdown 文档 | 抽取代码证据，按规模动态规划多篇文档，并生成 README 摘要 |

### jira

| 公式 | 适用场景 | 关键能力 |
| --- | --- | --- |
| `jira-bug-fix` | 从 Jira ticket 直接拉数据并嵌套 `bug-fix` | `[preflight]` 校验 `jira` CLI；第一步用 `script` step 调 `jira issue view --comments 20 --raw` 取结构化数据 |

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
| `git`（command） | `github-pr-rebase-main` / `github-pr-fix-comments` / `github-my-prs-rebase-main` / `code-docs` / `run-validation` |
| `git` + `require_repo = true` | `github-pr-review` / `gongbu` / `run-validation` |
| `jq`（command） | `run-validation` / `github-list-my-prs` / `github-fetch-pr` 等 |
| `python3`（command） | `code-docs` / `gongbu` |
| `bash`（command） | `run-validation` |
| `jira`（command） | `jira-bug-fix` |

如果 `tt formula run <name>` 报"preflight 失败"，直接对照这张表确认是否需要安装 / 登录。

## 推荐起点

- 写"批处理 + 改写 + commit"类工作流：从 `github-pr-rebase-main` 拷贝。
- 写"动态澄清 + 调研 + 决策"：从 `bug-fix` 拷贝。
- 写"知识整理 / 文档生成"：从 `fresh-topic-docs` 拷贝。
- 写"代码 feature 端到端"：默认从 `feature` 拷贝；如果需要隔离 worktree 与按需提交，再参考 `gongbu`。
- 写"PR / Jira 自动化"：从 `github-pr-review` / `jira-bug-fix` 拷贝，然后嵌入对应的 atomic 子步骤。

## 目录组织原则

- `formulas/github/`: GitHub PR 批处理、审阅、rebase、评论修复。
- `formulas/engineering/`: 通用代码实现、修 bug、冲突解决、Jira bug 修复。
- `formulas/docs/`: 代码或主题文档生成。
- `formulas/workflow/`: 方法论 / 决策型流程。
- `formulas/examples/`: 示例或集成演示。
- `atomics/github/`, `atomics/validation/`: 可被父 formula 复用的最小原子步骤。

路径只影响维护组织，运行时仍按 TOML 中的 `formula = "..."` 名称加载。

## 用户公式优先级

- 用户目录中的 `.toml` 优先于 builtin 同名公式（[推测]）—— 这与 `internal/formula/builtin.go` 把 builtin 注入到下层 namespace 的常见做法一致；同名时本地覆盖，便于就地修改而无需升级 `tt`。
- 想临时回到 builtin：`tt formula run --builtin <name>` 形式未在当前 CLI 中暴露（[推测]）；如需使用，复制到本地再 `tt formula copy`。

## 自我修复与 builtin

builtin / atomic 中的 `script` step 默认 `idempotent = false`，runtime 不会自动重试。要让某个 command（如 `gh pr view`、`git status`、`jq …` 这类只读命令）参与 `StepFixer` 自我修复，必须在 fork 后显式写 `idempotent = true`。详见 [Step Kinds 参考 · 通用字段](./step-kinds-reference.md#通用字段) 与 [Formula 系统 · 自我修复](./formula-system.md#自我修复self-repair)。
