# Builtin Atomic Formulas

`atomics/` contains small, composable builtin formulas. They are intended to be embedded by higher-level workflow formulas rather than used as long user-facing workflows.

## Authoring contract

Atomic formulas should follow these rules:

- **Single responsibility**: one atomic formula should do one deterministic job, such as fetch PR metadata, check git status, run validation, or push a branch.
- **Stable JSON output**: every important result should be available from a predictable step id, usually `*.stdout` when the step is a script.
- **No noisy reports by default**: atomics should avoid long Markdown reports unless the atomic genuinely needs human-facing synthesis.
- **No duplicated output**: each step should output only its own delta. Do not copy upstream raw JSON/stdout/stderr/diffs/logs into downstream outputs.
- **Explicit side effects**: formulas that mutate git state or remote state must say so in their description and should expose dry-run or caller-controlled flags where useful.
- **Composable failures**: expected external failures should normally become `ok:false` / `success:false` JSON instead of crashing the workflow, so parent formulas can decide what to do.

## Atomic catalog

| Formula | Category | Primary step | Purpose | Side effects |
|---|---|---:|---|---|
| `git-status-check` | git | `status.stdout` | Collect current repo status: branch, HEAD, upstream, dirty state, conflict files, and in-progress git operations. | Read-only |
| `git-run-validation` | git | `validation.stdout` | Run an explicit validation command in a repo, or skip successfully when no command is provided. | Runs caller command |
| `git-auto-detect-validation` | git | `validation.stdout` | Pick a lightweight validation command from repo markers such as `go.mod`, `package.json`, `pnpm-lock.yaml`, or `pyproject.toml`. | Runs detected/caller command |
| `git-integrate-ref` | git | `verify-integrated.stdout` plus `final-report` | Integrate a target ref into the current branch via rebase/merge and let an agent resolve conflicts if needed. Does not push. | Mutates local git history/worktree |
| `git-push-branch` | git | `push.stdout` | Push current `HEAD` to a remote branch with configurable `--force-with-lease`, `--no-verify`, and dry-run. | Remote push unless `dry_run=true` |
| `github-fetch-pr` | github | `pr.stdout` | Fetch GitHub PR metadata using `gh pr view`, with retry and REST fallback for numeric PR refs. | Read-only network call |
| `github-fetch-pr-files` | github | `files.stdout` | Fetch GitHub PR changed files using `gh pr view --json files`. | Read-only network call |
| `github-fetch-pr-diff` | github | `diff.stdout` | Fetch GitHub PR patch diff using `gh pr diff --patch`. | Read-only network call |
| `github-build-pr-context` | github | `context.stdout` | Normalize PR metadata, files, diff size, and review options into a compact context object. | Read-only |
| `github-list-my-prs` | github | `prs.stdout` | List open PRs for an author in the current repo or `repo_hint`. | Read-only network call |
| `repo-prepare-local-or-clone` | repo | `repo.stdout` | Resolve a local path, GitHub URL, or `owner/repo` into a local repo path, shallow-cloning when needed. | May clone into `.tt/tmp/repos` |
| `repo-evidence-map` | repo | `evidence.stdout` | Scan README/config/entry/test/source files into a lightweight evidence map with excerpts. | Read-only |

## Details

### `git-status-check`

Use this as the first git safety check in parent workflows.

Variables:

- `require_clean`: when `true`, `ok=false` if the worktree is dirty or a merge/rebase/cherry-pick is in progress.
- `require_upstream`: when `true`, `ok=false` if the current branch has no upstream.

Output: `status.stdout`

Key fields:

- `ok`, `reason`
- `repo_root`, `branch`, `head`, `upstream`
- `dirty`, `status_short`
- `in_rebase`, `in_merge`, `in_cherry_pick`
- `conflict_files`

### `git-run-validation`

Runs a caller-provided command and packages the result as JSON.

Variables:

- `repo_path`: optional repo path. Empty means current git root.
- `command`: validation command. Empty or `-` skips and returns `success=true`.
- `timeout_hint`: documentation/report hint only. The step timeout is fixed at `20m`.

Output: `validation.stdout`

Key fields:

- `requested`, `success`, `command`, `repo_root`
- `stdout` on success
- `stderr` on failure

### `git-auto-detect-validation`

Chooses a lightweight validation command when the caller does not provide one.

Detection order:

1. `go.mod` -> `go test ./...`
2. `package.json` -> `npm test`
3. `pnpm-lock.yaml` -> `pnpm test`
4. `pyproject.toml` -> `pytest`

Variables:

- `command`: optional override. When set, runs through `bash -lc`.

Output: `validation.stdout`

Key fields:

- `attempted`, `success`, `command`, `exit_code`
- `stdout` on success
- `stderr` on failure or skip reason

### `git-integrate-ref`

Ensures the current branch contains `target_ref`, using rebase or merge. This atomic can invoke an agent for conflict resolution and continuation. It intentionally **does not push**.

Variables:

- `target_ref`: ref to integrate, default `origin/main`.
- `strategy`: `rebase`, `merge`, or `auto`. `auto` currently resolves to `rebase`.
- `validation_command`: optional post-integration validation.
- `max_agent_rounds`: instruction limit for the agent loop.

Important steps:

- `pre-status.stdout`: initial repo suitability and target ref status.
- `attempt-integrate.stdout`: deterministic rebase/merge attempt result.
- `agent-integrate-loop`: agent JSON summary when deterministic integration fails.
- `verify-integrated.stdout`: final deterministic success criteria.
- `run-validation.stdout`: optional validation result.
- `final-report`: compact human-facing summary.

Side effects:

- May rebase or merge the current branch.
- May edit files and create commits during conflict resolution.
- Never pushes.

Parent workflow guidance:

- Use `git-status-check` before embedding if you need a stricter clean-worktree gate.
- Use `git-push-branch` explicitly after successful integration when remote mutation is desired.

### `git-push-branch`

Pushes current `HEAD` to a target remote branch. This is intentionally a separate atomic because it mutates remote state.

Variables:

- `remote`: default comes from upstream remote, then `origin`.
- `branch`: default comes from upstream branch, then current branch.
- `force_with_lease`: default `true`.
- `no_verify`: default `true`.
- `dry_run`: default `false`.

Output: `push.stdout`

Key fields:

- `requested`, `pushed`, `dry_run`
- `command`, `repo_root`, `remote`, `branch`, `head`
- `stdout`, `stderr`, `error`

### `github-fetch-pr`

Fetches PR metadata via GitHub CLI.

Variables:

- `pr_ref`: PR number, URL, or branch ref.
- `repo_hint`: optional `owner/repo`. Empty uses the current `gh` repo context.

Output: `pr.stdout`

Key fields include GitHub's PR fields plus normalized helpers:

- `ok`, `error`, `attempts`
- `pr_ref`, `repo_hint`
- `number`, `title`, `body`, `author`, `url`, `state`, `isDraft`
- `baseRefName`, `headRefName`, `headRefOid`
- `changedFiles`, `additions`, `deletions`, `commits`

### `github-fetch-pr-files`

Fetches changed files for a PR.

Variables:

- `pr_ref`: PR number, URL, or branch ref.
- `repo_hint`: optional `owner/repo`.

Output: `files.stdout`

Key fields:

- `ok`, `error`, `command`
- `files`: GitHub CLI file objects

Expected failures are converted into `ok=false` JSON.

### `github-fetch-pr-diff`

Fetches patch diff text for a PR.

Variables:

- `pr_ref`: PR number, URL, or branch ref.
- `repo_hint`: optional `owner/repo`.

Output: `diff.stdout`

Key fields:

- `ok`, `patch`, `patch_chars`
- `stderr`, `error`, `command`

Parent workflow guidance:

- Do not pass large `patch` values directly to final reports.
- Prefer using `patch_chars` and a curated analysis/summarization step.

### `github-build-pr-context`

Normalizes outputs from `github-fetch-pr`, `github-fetch-pr-files`, and optionally `github-fetch-pr-diff` into a compact review context.

Variables:

- `meta_json`: PR metadata JSON, normally `{{github-fetch-pr.pr.stdout}}`.
- `files_json`: PR files JSON, normally `{{github-fetch-pr-files.files.stdout}}`.
- `patch_chars`: diff size, normally `{{github-fetch-pr-diff.diff.stdout.patch_chars}}`.
- `review_focus`: comma/Chinese-comma separated focus areas.
- `submit_review`: whether the parent review formula intends to submit comments.

Output: `context.stdout`

Key fields:

- `ready`, `error`
- `number`, `title`, `url`, `author`, `state`, `is_draft`
- `base_branch`, `head_branch`, `head_sha`
- `changed_files`, `changed_files_count`, `additions`, `deletions`
- `pr_summary`, `focus_areas`, `diff_overview`
- `review_constraints`, `submit_review`

### `github-list-my-prs`

Lists open PRs for an author.

Variables:

- `author`: GitHub login. Empty uses `@me` behavior through gh where supported by the current implementation.
- `state`: normally `open`.
- `repo_hint`: optional `owner/repo`.
- `limit`: maximum PR count.

Output: `prs.stdout`

Key fields:

- `ok`, `error`, `command`
- `items` / `prs`: PR list suitable for parent formula loops
- count/config fields depending on GitHub CLI output

### `repo-prepare-local-or-clone`

Turns a repo input into a local directory.

Accepted `repo` inputs:

- Empty: use current git repository.
- Existing local directory: use that directory.
- GitHub URL: shallow clone if not already present in `.tt/tmp/repos`.
- `owner/repo`: shallow clone from GitHub if needed.

Variables:

- `repo`: repo input.
- `tmp_root`: clone cache root, default `.tt/tmp/repos`.

Output: `repo.stdout`

Key fields:

- `ok`, `error`
- `repo_input`, `repo_path`, `repo_root`
- `cloned`, `reused`
- `slug`, `remote_url`

### `repo-evidence-map`

Builds a lightweight repository evidence map for documentation, review, or planning workflows.

Variables:

- `repo_path`: repository directory.
- `focus`: optional focus hint to carry into the output.

Output: `evidence.stdout`

Key fields:

- `ok`, `error`
- `repo_path`, `focus`
- `evidence_files`: up to 60 scored files with `path`, `score`, and a short `excerpt`

Parent workflow guidance:

- This atomic intentionally includes excerpts. Do not pass `evidence_files` directly to a final report if the parent workflow also has detailed analysis steps. Summarize or project first.

## Composition examples

### Check status, integrate main, validate, then push

```toml
[[steps]]
id = "status"
embed = "git-status-check"

[steps.embed_vars]
require_clean = "true"
require_upstream = "true"

[[steps]]
id = "integrate-main"
embed = "git-integrate-ref"
depends_on = ["status"]
condition = "status.status.stdout.ok == true"

[steps.embed_vars]
target_ref = "origin/main"
strategy = "rebase"
validation_command = "go test ./..."

[[steps]]
id = "push"
embed = "git-push-branch"
depends_on = ["integrate-main"]
condition = "integrate-main.verify-integrated.stdout.success == true"

[steps.embed_vars]
dry_run = "false"
```

### Build a PR review context

```toml
[[steps]]
id = "pr"
embed = "github-fetch-pr"
[steps.embed_vars]
pr_ref = "{{pr_ref}}"
repo_hint = "{{repo_hint}}"

[[steps]]
id = "files"
embed = "github-fetch-pr-files"
[steps.embed_vars]
pr_ref = "{{pr_ref}}"
repo_hint = "{{repo_hint}}"

[[steps]]
id = "diff"
embed = "github-fetch-pr-diff"
[steps.embed_vars]
pr_ref = "{{pr_ref}}"
repo_hint = "{{repo_hint}}"

[[steps]]
id = "context"
embed = "github-build-pr-context"
depends_on = ["pr", "files", "diff"]

[steps.embed_vars]
meta_json = "{{pr.pr.stdout}}"
files_json = "{{files.files.stdout}}"
patch_chars = "{{diff.diff.stdout.patch_chars}}"
review_focus = "正确性、风险与可维护性"
submit_review = "false"
```

## Maintenance checklist

When adding or changing an atomic formula:

1. Add top-level `formula`, `description`, `version`, `type = "atomic"`, `category`, and `tags`.
2. Prefer one primary step with a stable id and JSON output.
3. Add `description` to every step.
4. Add preflight checks for required CLIs and auth.
5. Convert expected external failures into stable JSON fields.
6. Keep stdout compact. If a value can be huge, expose size/manifest fields and let parent workflows decide whether to read the full content.
7. Document variables, output step, key fields, and side effects in this README.
8. Validate with `go test ./internal/formula -run 'TestBuiltin|TestAllBuiltin'` and `go run . formula compile <formula>`.
