# Builtin Atomic Formulas

`atomics/` 只放可以被上层 builtin formula 复用的原子步骤。它们不是独立产品化 workflow，单独运行通常没有意义。

## 保留原则

- **必须被复用**：只有当前 builtin formulas 通过 `embed = "..."` 使用的流程才放在这里。
- **原子职责**：一个 atomic 只做一个稳定步骤，例如获取 PR metadata、获取 PR diff、运行验证。
- **稳定 JSON 契约**：输出必须适合父 formula 通过 `xxx.stdout.field` 消费。
- **少报告，多数据**：atomic 默认不做长 Markdown final report，只输出结构化数据。
- **不重复输出**：每个 step 只输出自己的增量，不复制上游 JSON/stdout/stderr/diff/log。
- **清爽优先**：如果一个 atomic 暂时没有 builtin formula 使用，就删除。未来有真实复用需求时再加回来。

## 当前保留清单

| Atomic | 被谁使用 | 主输出 | 用途 |
|---|---|---|---|
| `git-auto-detect-validation` | `feature`, `gongbu` | `validation.stdout` | 根据仓库标记自动选择轻量验证命令并执行 |
| `git-run-validation` | `github-pr-rebase-main` | `validation.stdout` | 在指定 repo 运行父流程传入的验证命令 |
| `github-fetch-pr` | `github-pr-review`, `github-pr-fix-comments`, `github-pr-rebase-main` | `pr.stdout` | 一次获取单个 GitHub PR 的 metadata、changed files、patch diff 和 review 上下文字段 |
| `github-list-my-prs` | `github-my-prs-fix-comments`, `github-my-prs-rebase-main` | `prs.stdout` | 列出当前仓库指定作者的 open PR，供批量流程循环 |

## 详细说明

### `git-auto-detect-validation`

父流程：`feature`, `gongbu`

变量：

- `command`: 可选覆盖验证命令；为空则自动检测。

检测顺序：

1. `go.mod` -> `go test ./...`
2. `package.json` -> `npm test`
3. `pnpm-lock.yaml` -> `pnpm test`
4. `pyproject.toml` -> `pytest`

输出：`validation.stdout`

关键字段：

- `attempted`
- `success`
- `command`
- `exit_code`
- `stdout`
- `stderr`

### `git-run-validation`

父流程：`github-pr-rebase-main`

变量：

- `repo_path`: 可选仓库目录；为空时使用当前 git root。
- `command`: 验证命令；为空或 `-` 时跳过并返回 `success=true`。
- `timeout_hint`: 仅给报告使用，实际 step timeout 固定为 `20m`。

输出：`validation.stdout`

关键字段：

- `requested`
- `success`
- `command`
- `repo_root`
- `stdout`
- `stderr`

### `github-fetch-pr`

父流程：`github-pr-review`, `github-pr-fix-comments`, `github-pr-rebase-main`

这是唯一的“获取单个 PR” atomic。它一次调用 GitHub CLI 获取 metadata、changed files、patch diff，并派生 review 所需上下文字段。父 formula 需要什么字段就直接从 `pr.stdout` 取，不再拆成多个 fetch atomic。

变量：

- `pr_ref`: PR 编号、URL 或分支引用。
- `repo_hint`: 可选 `owner/repo`，为空时使用当前 `gh` repo context。
- `review_focus`: 审阅关注点，用于派生 `focus_areas`。
- `submit_review`: 父流程是否计划提交 review，仅作为上下文字段输出。

输出：`pr.stdout`

关键字段：

- `ok`, `ready`, `error`, `diff_error`
- `number`, `title`, `body`, `author`, `url`, `state`, `isDraft`
- `baseRefName`, `headRefName`, `headRefOid`
- `files`: GitHub CLI 原始 files 列表
- `patch`, `patch_chars`
- `changed_files`, `changed_files_count`
- `base_branch`, `head_branch`, `head_sha`
- `pr_summary`, `focus_areas`, `diff_overview`
- `review_constraints`, `submit_review`

注意：`patch` 可能很大，最终报告不要直接粘贴。父流程应只把它传给分析 step。

### `github-list-my-prs`

父流程：`github-my-prs-fix-comments`, `github-my-prs-rebase-main`

变量：

- `author`: GitHub login。
- `state`: 通常是 `open`。
- `repo_hint`: 可选 `owner/repo`。
- `limit`: 最大 PR 数量。

输出：`prs.stdout`

关键字段：

- `ok`
- `items` / `prs`
- `error`
- `command`

## 维护流程

新增 atomic 前先问两个问题：

1. 是否已经有至少一个 builtin formula 会立即 `embed` 它？
2. 它是否真的是一个可复用的原子步骤，而不是一次性 workflow？

如果答案不是两个都是 yes，不要放进 `atomics/`。

修改后必须验证：

```bash
go test ./internal/formula -run 'TestBuiltin|TestAllBuiltin'
```

并逐个 compile 当前 atomic：

```bash
go run . formula compile git-auto-detect-validation
go run . formula compile git-run-validation
go run . formula compile github-fetch-pr
go run . formula compile github-list-my-prs
```
