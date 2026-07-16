---
name: formula-writer
description: 'Design, write, refactor, validate, and troubleshoot tt formula TOML workflows. Use when creating or optimizing formula files, choosing step types, replacing agent work with deterministic tools/scripts, modeling data flow, branches, loops, human input, tool calls, aggregate projections, or embedded workflows.'
license: MIT
---

# Formula Writer

Use this skill to author **typed-runtime `tt formula` workflows**. A formula is not a long prompt. It is a repeatable operating procedure expressed as a typed workflow graph.

The core rule is:

> Use deterministic non-agent steps whenever the computer can do the work exactly. Use agents only for judgment, synthesis, planning, implementation reasoning, and user-facing explanation.

## Current architecture facts

- `tt formula run` uses the typed runtime and Workflow IR. Do not mention legacy runtime flags.
- Canonical formula path is `.tt/formulas/<name>.toml`. Do not create `.formula.toml` files.
- Step outputs are saved under the local step `id` by default. Use those ids as context keys.
- Every step follows one `inputs map -> step -> outputs map` contract. Reusable formulas declare their public result map with root `[outputs.<name>]` tables; `report` is the conventional human-readable Markdown port.
- A reusable workflow should normally be called at runtime with `execution = "formula"`, a static `formula = "child-name"`, and explicit `[steps.with]` input bindings. Use compile-time `embed` only when the caller must inline the child graph or consume child implementation steps that are intentionally not public outputs.
- Runtime injects global `env`: `env.cwd`, `env.os.name`, `env.os.arch`, `env.git.is_repo`, `env.git.root`, `env.git.repo`, `env.git.branch`, `env.git.commit`, `env.git.remote_url`.
- Runtime templates such as `{{topic}}`, `{{env.cwd}}`, and `{{some-step.field}}` can be used in descriptions, script argv/env/cwd, and tool config strings.
- Conditions use bare expressions, not templates: `condition = "classify.kind == bug"`, not `condition = "{{classify.kind}} == bug"`.
- External agent steps are available when a workflow should delegate to an installed CLI agent such as jcode, codex, opencode, forge, or bl. In recipe TOML use `execution = "external_agent"` plus `[steps.external_agent]`; in typed schema use `kind = "external_agent"` and top-level driver fields.
- **Self-Repair**: failed / validation-failed steps go through a `StepFixer` abstraction (`agentFixer` / `scriptFixer`) and may be retried up to 3 attempts. Whether a step may be retried is controlled by the `idempotent` flag (recipe TOML: top-level `idempotent = true|false`; typed schema: `StepDecl.Idempotent`). Defaults by kind:
  - `agent` / `external_agent` → `true` (LLM call, no persistent side effect)
  - `script` → `false` (shell may have side effects; must opt-in explicitly)
  - `tool` / `aggregate` / `write_files` / `noop` / `human_input` / `loop` / `retry` → not in the fix path
  - Repair reports are persisted to `patches/<run-id>.json` and surfaced in the dashboard `Repairs` panel with a `Confirm reviewed` flow; the runtime never auto-patches the formula file.

## Delivery contract

When asked to create or optimize a formula:

1. Prefer writing/updating a complete TOML file.
2. Include root fields: `formula`, `description`, `version`, `type = "workflow"`.
3. For a formula intended to be reused, declare a small stable `[outputs]` contract. Always expose `outputs.report` when the formula produces a final user-facing report; expose curated machine-readable results, not internal planning/research steps.
4. Use short stable local step ids: `fetch-pr`, `classify`, `run-tests`, `report`.
5. Make execution order explicit with `depends_on`.
6. Make data flow explicit with `input_context` for agent consumers, but only pass data the agent actually needs.
7. Prefer deterministic `tool`, `aggregate`, or `script` steps before agent steps.
8. Use `[steps.validate]` for JSON that drives downstream structure, conditions, loops, or tools.
9. Use top-level `[preflight]` for required CLI/env/repo prerequisites; do not model prerequisite checks as workflow steps unless their output is needed downstream.
10. Keep the workflow product-facing: internal safety constraints belong in step instructions, not in user-facing final reports.
11. Validate with `tt formula validate`, `tt formula compile`, and, when safe, `tt formula run --dry-run`.

## Methodology: design before TOML

### 1. Write the SOP in plain language

Example:

```text
Input: PR number.
Fetch PR metadata deterministically.
Fetch current git branch deterministically from env.
Ask coder agent to review risk.
Run tests deterministically.
Report findings.
```

If a step says “figure out by looking around”, split it into deterministic collection plus agent judgment.

### 2. Classify every action

Ask this in order:

| Question | Use |
|---|---|
| Is it a built-in deterministic operation like sleep/write files/git fetch/git push/git branch/git checkout? | `execution = "tool"` |
| Is it data shaping/projection/manifest creation from prior outputs? | `execution = "aggregate"` |
| Is it a local command/API call not covered by ToolStep? | `execution = "script"` |
| Is it a known user approval/choice/private value? | `execution = "human_input"` |
| Is missing context only sometimes needed and should be decided at runtime? | agent step with `form = true` |
| Should this step run through an external installed agent CLI instead of the embedded Picoclaw agent? | `execution = "external_agent"` with `[steps.external_agent]` |
| Does it require judgment, tradeoff, synthesis, code reasoning, or prose? | agent step, omit `execution` |
| Is it repeat-until or foreach runtime iteration? | `[steps.loop]` |
| Is it a stable reusable workflow with an explicit public input/output contract? | `execution = "formula"` + `formula = "child-formula"` |
| Must the caller inline the child graph or consume non-public child step outputs? | `embed = "child-formula"` |

### 3. Draw the data flow

Use step ids as data contracts:

```text
fetch-data -> aggregate-manifest -> write-files -> report
```

Rules:

- `depends_on` controls order only. Add a dependency only when the step cannot safely start until that upstream step is complete.
- `input_context` controls what an agent sees. It is not a logging mechanism and is not a substitute for dependencies.
- Agent dependencies and `input_context` are not "more is better". They must be minimal, intentional, and reviewable.
- For every `depends_on` edge, be able to answer: "what race or missing side effect would happen without this edge?" If there is no concrete answer, remove it.
- For every `input_context` item, be able to answer: "what decision, output field, or user-facing statement requires this exact data?" If there is no concrete answer, remove it.
- Prefer precise field paths in `input_context` when an agent only needs a few fields, such as `fetch-pr.stdout.title` or `summary.stdout.changed_files`, rather than an entire raw step output.
- Prefer whole step ids only when the step output is intentionally small and curated for that downstream agent.
- Do not pass credentials, raw logs, full diffs, large stdout, broad scan results, or complete previous agent outputs to another agent unless that exact content is necessary.
- Before a final report, strongly prefer a deterministic `summarize-*` script/aggregate step that builds a small report payload. The reporter should consume that payload, not the whole workflow transcript.
- Do not feed huge content to an agent if a deterministic step can shrink or materialize it first.
- Treat each step output as a delta contract: include only information unique to that step. Do not repeat upstream fields unless a downstream `condition`, `loop.until`, `aggregate`, or report explicitly needs them.
- For final reports, pass curated summaries or manifests, not full raw stdout/stderr/diffs/logs. A reporter must synthesize conclusions instead of pasting upstream JSON or child reports.

### 4. Keep the formula simple and product-facing

Design from the user's mental model, not from implementation plumbing. A formula should feel like one repeatable capability, not a transcript of every internal helper.

Rules:

- Do not expose internal safety constraints in the final report unless they materially affect the user's next action. Put constraints like "do not push", "do not checkout", "do not continue rebase", or "only touch these files" in step instructions, not in the user report.
- The final report should answer user-facing questions: what happened, what files/items changed, what remains blocked, what validation ran, what to do next.
- Treat every `title`, step `description`, `[steps.form].title`, `[steps.form].description`, `submit_label`, field `label`, field `help`, and `placeholder` as product UI copy when it can appear in the web/dashboard/runtime UI. Do not put authoring notes, agent instructions, implementation reasons, or meta explanations there.
- For `human_input`, separate internal reason from user copy: the step prompt/design may know "why clarification is needed", but the visible form should simply tell the user what to do next in natural language. Avoid phrases like "the previous step decided...", "use radio/checkbox...", "collect constraints...", or "this is required for downstream agent".
- Not every upstream step belongs in `final-report.input_context`. Prefer one curated summary/report-data step plus optional validation output.
- If multiple deterministic script steps are only mechanical plumbing and no downstream agent/user needs their intermediate outputs, combine them into one step.
- Split steps only when it improves retry/failure semantics, enables branching/looping, provides a meaningful UI milestone, produces a reusable data contract, or separates deterministic collection from agent judgment.
- Avoid "placeholder" steps that only say something was skipped. If a behavior is intentionally disabled, remove the step or encode that in the summary data only when the user needs to know.
- For side-effect workflows, keep the boundary narrow: operate only on explicitly scoped inputs. Do not run broad commands like `git add -u`, package-manager regeneration, cleanup, checkout/reset, or push unless the formula's explicit user-facing purpose requires it.

Preferred reporting pattern:

```text
collect/act deterministic steps -> summarize-result JSON -> final-report agent
```

The reporter should usually consume:

```toml
input_context = ["summarize-result.stdout", "run-validation.stdout"]
```

instead of every raw upstream step.

### 5. Move deterministic work out of agents

Do **not** ask an agent to:

- create directories or write generated files when a tool/script can do it;
- run `git fetch`, `git checkout`, `git push`, or branch operations;
- sleep/wait a fixed duration;
- parse/filter JSON that `aggregate` can project;
- run tests or CLI commands;
- validate that a file exists if a command/tool can check it.

Agent steps should receive curated facts and produce judgments/reports.

## External agent steps

Use `external_agent` only when the user explicitly wants another installed agent CLI, or when a task depends on that tool's behavior/session. Prefer normal embedded agent steps otherwise.

```toml
[[steps]]
id = "codex-review"
title = "External review with Codex"
execution = "external_agent"
description = "Review the collected evidence and return concise risks."
depends_on = ["collect-evidence"]
input_context = ["collect-evidence"]

[steps.external_agent]
driver = "codex" # jcode | codex | opencode | forge | bl
model = "gpt-5"
mode = "exec"
timeout = "5m"
extra_args = ["--sandbox", "read-only"]
```

Notes:

- `tt formula run --ext-driver <driver>` sets the default driver when a step omits `driver`.
- `bl` is routed through `jcode run --provider bl`.
- Always add a matching `[preflight]` command check for non-standard CLIs such as `codex`, `opencode`, or `forge`.
- `codex` resume maps to `codex exec resume <session>`. `forge` maps to top-level `forge --prompt`; let forge use the provider/model configured during installation or setup; do not add a generic forge `--model` flag.
- `external_agent` defaults to `idempotent = true`; opt out with `idempotent = false` only if a particular driver/install has side effects (rare).

## Minimal formula skeleton

```toml
formula = "example-workflow"
description = "Short repeatable workflow description."
version = 1
type = "workflow"

[preflight]

[[preflight.checks]]
type = "command"
name = "git"
command = "git"
message = "git is required"

[vars]
topic = { description = "Thing to process", required = true }

[[steps]]
id = "analyze"
title = "Analyze {{topic}}"
description = "Analyze the topic and output concise findings."

[steps.agent]
name = "planner"
```

## Preflight checks

Use top-level `[preflight]` for prerequisites that must be true before the workflow starts, such as required CLIs, auth commands, environment variables, paths, or git repository state. Preflight checks are not workflow nodes and their output is not available through `input_context`. Do **not** create a `*-preflight` step just to check whether a CLI exists.

Supported check types:

| Type | Required fields | Purpose |
|---|---|---|
| `command` | `command` | Check an executable exists in `PATH` via `exec.LookPath`. |
| `exec` | `command` or `command` + `args` | Run a shell command before the formula starts, for auth/config checks such as `gh auth status` or `jira --help`. |
| `git` | `require_repo`, `require_remote` | Require the workspace to be a git repo and/or have a remote. |
| `env` | `env` or `name` | Require an environment variable to be set. |
| `path` | `path` | Require a file or directory path to exist. Relative paths are resolved from the workspace. |

Every preflight check may set `condition` using compile-time variable syntax, e.g. `condition = "{{driver}} == codex"`. Formula defaults apply first and `--var driver=...` overrides them at run time.

Example:

```toml
[preflight]

[[preflight.checks]]
type = "command"
name = "jira"
command = "jira"
message = "Install and configure Jira CLI before running this formula."

[[preflight.checks]]
type = "command"
name = "codex"
command = "codex"
condition = "{{driver}} == codex"
message = "Install Codex CLI when driver=codex."

[[preflight.checks]]
type = "exec"
name = "jira-callable"
command = "jira --help"
message = "Jira CLI is installed but not callable; check auth/configuration."
```

## Step creation guide

### 1. Agent step, for judgment and synthesis

Default when `execution` is omitted.

Use for: classification, planning, code reasoning, product tradeoffs, report writing, summarization.

```toml
[[steps]]
id = "classify"
title = "Classify request"
description = """
Classify the request.
Output ONLY compact JSON:
{"kind":"bug|feature|question","confidence":0.0,"reason":"..."}
"""

[steps.agent]
name = "planner"

[steps.validate]
format = "json"
required = ["kind", "confidence", "reason"]
```

Agent guidance:

- Use `planner` for decomposition/strategy.
- Use `coder` for code investigation/implementation reasoning.
- Use `tester` for validation strategy.
- Use `writer` or `reporter` for user-facing prose.
- If output drives `condition`, `loop.until`, `aggregate`, or tools, require compact JSON and validate it.
- Do not mix long Markdown and control JSON in one step. Split them.
- For reporter/writer steps, explicitly forbid repeating raw upstream JSON, stdout/stderr, diffs, logs, or full child reports. Ask for only each upstream step's unique conclusion, state change, files/URLs/commits, risks, and next action.

### 2. Tool step, for built-in deterministic operations

Use ToolStep for common computer actions. Current built-ins:

- `write_files`: create directory and write many files from JSON objects.
- `sleep`: wait a fixed duration.
- `git_fetch`: run `git fetch`.
- `git_push`: run `git push`.
- `git_branch`: list/create/delete branches.
- `git_checkout`: checkout or create branch.
- `git_worktree`: create/list/remove git worktrees for isolated feature development. For large repos, set `sparse_paths = ["cmd", "internal/pkg"]`; this uses `git worktree add --no-checkout`, `git sparse-checkout set`, then `git checkout` so only the requested paths are populated. Optional `sparse_mode` is `cone` (default) or `no-cone`.

General shape:

```toml
[[steps]]
id = "pause"
title = "Wait before polling"
execution = "tool"

[steps.tool]
name = "sleep"

[steps.tool.sleep]
duration = "5s"
# or seconds = 5
```

Write files from generated JSON:

```toml
[[steps]]
id = "write-doc-files"
title = "Write generated docs"
depends_on = ["generate-docs", "package-plan"]
execution = "tool"

[steps.tool]
name = "write_files"

[steps.tool.write_files]
source = "generate-docs"
root = "docs"
dir_name = "{{package-plan.topic_name}}"
# defaults: filename_key="filename", title_key="title", summary_key="summary", content_key="content"

[steps.validate]
format = "json"
required = ["directory", "files"]
```

Git tools:

```toml
[[steps]]
id = "fetch-origin"
title = "Fetch origin"
execution = "tool"
[steps.tool]
name = "git_fetch"
[steps.tool.git_fetch]
remote = "origin"
prune = true
```

```toml
[[steps]]
id = "checkout-branch"
title = "Create and checkout branch"
execution = "tool"
[steps.tool]
name = "git_checkout"
[steps.tool.git_checkout]
branch = "feature/{{ticket}}"
create = true
start_point = "origin/main"
```

Create an isolated worktree for feature work:

```toml
[[steps]]
id = "create-feature-worktree"
title = "Create isolated feature worktree"
execution = "tool"

[steps.tool]
name = "git_worktree"

[steps.tool.git_worktree]
path = "../repo-{{ticket}}"
branch = "feature/{{ticket}}"
create = true
start_point = "origin/main"
```

Create a sparse worktree for very large repositories. Use this when the task only needs a few directories:

```toml
[[steps]]
id = "create-sparse-worktree"
title = "Create sparse feature worktree"
execution = "tool"

[steps.tool]
name = "git_worktree"

[steps.tool.git_worktree]
path = "../repo-{{ticket}}"
branch = "feature/{{ticket}}"
create = true
start_point = "origin/main"
sparse_paths = ["cmd", "internal/formula", "docs"]
# sparse_mode defaults to "cone"; use "no-cone" only when you need gitignore-style patterns.
sparse_mode = "cone"
```

List or remove worktrees:

```toml
[[steps]]
id = "list-worktrees"
title = "List worktrees"
execution = "tool"
[steps.tool]
name = "git_worktree"
[steps.tool.git_worktree]
list = true
porcelain = true

[[steps]]
id = "remove-feature-worktree"
title = "Remove feature worktree"
execution = "tool"
[steps.tool]
name = "git_worktree"
[steps.tool.git_worktree]
remove = "../repo-{{ticket}}"
force = true
```

```toml
[[steps]]
id = "push-branch"
title = "Push branch"
execution = "tool"
[steps.tool]
name = "git_push"
[steps.tool.git_push]
remote = "origin"
branch = "feature/{{ticket}}"
set_upstream = true
```

ToolStep rules:

- Use tools before script when a built-in exists.
- Tools are deterministic and auditable.
- Tool outputs are JSON and can be consumed by later agent steps.
- Keep `input_context` minimal. If a downstream step only needs one field from a JSON output, pass the field path such as `input_context = ["inspect-repo.stdout"]`; pass the whole step id only when multiple fields or the full JSON object are needed.
- Do not add `output_key` for normal formulas. A step's `id` is already the output key.
- Avoid adding a new execution kind for every operation. Prefer adding a new `tool.name`.

### 3. Aggregate step, for JSON projection and fan-in manifests

Use `aggregate` to collect objects from prior outputs and include/exclude fields. This is how to reduce token usage before an agent step.

```toml
[[steps]]
id = "article-manifest"
title = "Build article manifest"
depends_on = ["write-articles"]
execution = "aggregate"

[steps.aggregate]
source = "write-articles"
as = "articles"
require = ["filename", "title", "summary", "content"]
exclude = ["content"]

[steps.validate]
format = "json"
required = ["articles"]
```

Use aggregate when:

- downstream agent needs filenames/summaries but not large content;
- multiple loop outputs need a manifest;
- you need to extract only stable fields before reporting;
- you would otherwise write Python just to walk/filter JSON.

### 4. Script step, for deterministic commands not yet tools

Use script when no ToolStep exists and a local command/API is the right abstraction.

```toml
[[steps]]
id = "run-tests"
title = "Run tests"
execution = "script"

[steps.script]
command = ["go", "test", "./..."]
format = "text"
timeout = "5m"
continue_on_error = true
```

Script rules:

- Prefer argv: `command = ["gh", "pr", "view", "{{pr}}", "--json", "number,title"]`.
- Use `bash -lc` only for short glue when necessary.
- Avoid Python for routine filesystem, git, sleep, or JSON projection. Prefer `tool` or `aggregate`.
- Always set `timeout` for non-trivial commands.
- Use `continue_on_error = true` when failure should become report data.
- Avoid destructive commands: `rm -rf`, `sudo`, `chmod`, `chown`, `dd`, `mkfs`, shutdown/reboot, pipe-to-shell.
- **Idempotency**: `script` defaults to `idempotent = false`. For commands that are safe to re-run (read-only commands like `gh`, `curl GET`, `jq`, `git status`/`git diff`/`git fetch`, `go test`), add `idempotent = true` to enable the runtime's `scriptFixer` self-repair (up to 3 attempts, persisted as `RepairRecord`). Leave the default for side-effecting commands (`git push`, `gh pr create`, anything that writes).

### 5. Static human input step, for known gates

Use when the workflow must pause at this exact point.

```toml
[[steps]]
id = "approve-release"
title = "Approve release"
execution = "human_input"

[steps.form]
title = "Approve release"
description = "Choose whether to continue."
submit_label = "Continue"

[[steps.form.fields]]
name = "approved"
label = "Approved?"
type = "radio"
required = true
options = ["yes", "no"]
```

### 6. Dynamic clarification, for unknown missing context

Use `form = true` on an agent step when the agent should decide whether user input is required.

```toml
[[steps]]
id = "triage"
title = "Triage and clarify if needed"
form = true
description = """
Analyze the request. If safe progress is blocked by missing info, ask the minimum dynamic form.
Otherwise output ONLY compact JSON:
{"ready":true,"summary":"...","assumptions":[]}
"""

[steps.agent]
name = "planner"

[steps.validate]
format = "json"
required = ["ready", "summary"]
```

### 7. Runtime loop step

Use loops for runtime iteration, not for static decomposition.

```toml
[[steps]]
id = "improve"
title = "Improve until approved"
depends_on = ["draft-brief"]

  [steps.loop]
  until = "review.approved == true"
  max = 3

  [[steps.loop.body]]
  id = "draft"
  title = "Draft iteration {{iteration}}"
  input_context = ["draft-brief"]
  description = "Create or improve the draft."

  [steps.loop.body.agent]
  name = "writer"

  [[steps.loop.body]]
  id = "review"
  title = "Review iteration {{iteration}}"
  depends_on = ["draft"]
  input_context = ["draft"]
  description = "Output ONLY compact JSON: {\"approved\":true,\"reason\":\"...\"}."

  [steps.loop.body.agent]
  name = "tester"

  [steps.loop.body.validate]
  format = "json"
  required = ["approved", "reason"]
```

Loop rules:

- Always set `max` for `until` loops.
- The `until` condition references outputs from loop body step ids.
- Use `for_each` for arrays produced at runtime.
- Keep loop body small and inspectable.

### 8. Public Formula outputs and runtime FormulaCall

Declare a small stable output contract on every formula intended for reuse. `from` is a runtime context path. Prefer a curated aggregate such as `report-data` over exporting internal research, planning, or implementation steps directly.

```toml
[outputs.report]
from = "final-report"
type = "markdown"
required = true
description = "Human-readable final report"

[outputs.summary]
from = "report-data.summary"
type = "string"
required = true
description = "Stable machine-readable result summary"
```

Call that formula as one normal runtime step:

```toml
[[steps]]
id = "run-child"
title = "Run child workflow"
execution = "formula"
formula = "child-workflow"

[steps.with]
request = "{{request}}"          # exact template preserves JSON/object/array type
label = "ticket={{ticket_key}}" # mixed template is a string

[[steps]]
id = "report"
title = "Report child result"
depends_on = ["run-child"]
input_context = ["run-child.report", "run-child.summary"]
description = "Summarize the child result."
```

FormulaCall rules:

- `[steps.with]` is the explicit child input map. An exact template such as `{{payload}}` preserves the original value type; mixed text such as `id={{ticket}}` produces a string; text without a template is a literal string, not an implicit context lookup.
- Child results are exposed only through its declared `[outputs.<name>]` map and are read as `<call-step-id>.<output-name>`. Do not depend on child internal step ids.
- The target formula name is static. `validate` and `compile` link the complete call graph and reject missing targets, unknown or missing required inputs, undeclared output references, cycles, and excessive nesting.
- A top-level `final-report` still provides a compatibility `report` output when no explicit declaration exists, but reusable formulas should declare it explicitly so the interface is reviewable.
- FormulaCall inside a parallel loop is rejected unless the call explicitly sets `allow_parallel = true`. Opt in only when child side effects and workspace usage are isolated or read-only.

### 9. Embed step, only for intentional graph inlining

```toml
[[steps]]
id = "bug-fix"
title = "Run bug-fix workflow"
embed = "bug-fix"

[steps.embed_vars]
issue_summary = "{{triage.issue_summary}}"
```

Embed rules:

- Use embed only when the caller must merge the child nodes into its own graph, depend on a child implementation node, or reuse an atomic implementation fragment that intentionally has no public contract.
- If the caller only needs declared child results, use FormulaCall instead.
- Do not mix `embed` with loop, script, tool, agent, form, or children on the same step.
- Downstream dependencies wait for the embedded workflow exit boundary.

### 10. Noop step, only for real graph structure

```toml
[[steps]]
id = "join"
title = "Join parallel branches"
execution = "noop"
depends_on = ["frontend-check", "backend-check"]
```

Use sparingly. Do not add noop boundaries just because old systems needed them.

## Validation rules

Use `[steps.validate]` for structured outputs.

Object:

```toml
[steps.validate]
format = "json"
required = ["kind", "confidence"]
```

Array of objects:

```toml
[steps.validate]
format = "json"
min_items = 1
item_required = ["filename", "title"]
```

Validation accepts direct JSON and agent text containing JSON/fenced JSON, but still instruct agents to output ONLY compact JSON for reliability.

## Conditions

Good:

```toml
condition = "classify.kind == bug"
condition = "env.git.is_repo == true"
condition = "review.approved != true"
```

Bad:

```toml
condition = "{{classify.kind}} == bug"
```

## Recommended workflow shapes

### Deterministic first

```text
tool/script collect facts -> aggregate/project -> agent judgment -> tool/script validate -> agent report
```

### Large generated content

```text
agent generate content -> tool write_files -> aggregate manifest -> agent README/report
```

Never pass all article bodies to a reporting agent if a manifest is enough.

### Git workflow

```text
git_fetch tool -> git_checkout tool -> agent implement/review -> script test -> git_push tool -> report
```

## Commands for validation

```bash
tt formula validate .tt/formulas/<name>.toml
tt formula compile <name> --dir .tt/formulas
tt formula run <name> --dir .tt/formulas --dry-run
```

`validate` and `compile` also resolve FormulaCall targets and verify their public input/output contracts. Run them for both a reusable child and at least one parent call chain after changing `[vars]`, `[outputs]`, or `[steps.with]`.

Saved runs:

```bash
tt formula runs --formula <name>
tt formula run show latest
tt formula run show latest --step <step-id>
tt formula run open latest
tt formula run resume latest
tt formula run input latest <step-id> --field key=value
```

## Final self-check

Before handing off:

1. Root fields are complete and filename is `<name>.toml`.
2. Every step id is unique, short, and local.
3. Reusable formulas declare stable public outputs, including `report` when applicable; callers consume only those public ports.
4. FormulaCall uses explicit `[steps.with]` bindings; embed is reserved for intentional inlining.
5. Dependencies reference existing step ids.
6. Deterministic operations use `tool`, `aggregate`, or `script`, not agent prose.
7. Agent steps have clear goal, inputs, output format, and constraints.
8. JSON outputs that control downstream behavior have validation.
9. Large content is materialized or projected before reporting.
10. Human input uses `human_input` or `form = true`, not “ask the user” prose.
11. Loops have `max` and inspectable bodies.
12. Validation/compile/dry-run commands pass.
13. Side-effecting `script` steps are left at the default (`idempotent = false`); safe read-only scripts opt in with `idempotent = true` so the runtime's `StepFixer` can retry them.

## Self-Repair (StepFixer)

When a step fails or its `validate` schema fails, the typed runtime can automatically retry it. The mechanism is a `StepFixer` registry, the retry loop lives in `executor.tryFixAndRerun`, and it caps at `maxFixAttempts = 3` (compile-time constant; per-step override is a future feature).

### Opt-in / opt-out

| Kind | Default `idempotent` | To enable Self-Repair |
| --- | --- | --- |
| `agent` | `true` | nothing required; set `idempotent = false` to opt out |
| `external_agent` | `true` | same as `agent` |
| `script` | `false` | add `idempotent = true` explicitly |
| `tool` / `aggregate` / `write_files` / `noop` / `human_input` / `loop` / `retry` | n/a (not in the fix path) | n/a |

Examples:

```toml
# Read-only script — opt in to self-repair
[[steps]]
id = "fetch-pr"
execution = "script"
idempotent = true

[steps.script]
command = ["gh", "pr", "view", "{{pr}}", "--json", "number,title,state"]
timeout = "30s"

# Side-effecting script — keep default (no self-repair)
[[steps]]
id = "push-branch"
execution = "script"
# idempotent = false   ← implicit; explicit write is also fine

[steps.script]
command = ["git", "push", "--force-with-lease", "origin", "HEAD"]
```

### What gets written

Each attempt produces a `RepairRecord` (fields: `StepID / Kind / Attempt / Status / Reason / Advice / FormulaUpdateHint / NextAttemptHint / OriginalCommand / FixedCommand / Error / RecordedAt / ConfirmedAt / ConfirmationStatus`). The list is:

- kept in `runtime.Snapshot.Repairs` for the dashboard;
- persisted to `patches/<run-id>.json` via `formularun.Store.SaveRepairs`;
- broadcast over the dashboard WebSocket as `step.repair.recorded` events.

### Author workflow after a run

1. Open the run, look at the dashboard `Repairs` panel.
2. Click `Confirm reviewed` on each entry once you have read the `FormulaUpdateHint` (or for `script` entries, the `OriginalCommand` → `FixedCommand` diff).
3. Manually apply the patch to the formula file (the runtime never writes the formula file). `tt formula create` / `optimize` can be used to regenerate cleanly.
4. Re-run the formula to confirm the fix.

Never declare `idempotent = true` for steps whose retry could corrupt state (writes, deletes, force pushes, stateful API calls). When in doubt, keep the default.
