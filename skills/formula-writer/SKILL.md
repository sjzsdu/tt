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
- Runtime injects global `env`: `env.cwd`, `env.os.name`, `env.os.arch`, `env.git.is_repo`, `env.git.root`, `env.git.repo`, `env.git.branch`, `env.git.commit`, `env.git.remote_url`.
- Runtime templates such as `{{topic}}`, `{{env.cwd}}`, and `{{some-step.field}}` can be used in descriptions, script argv/env/cwd, and tool config strings.
- Conditions use bare expressions, not templates: `condition = "classify.kind == bug"`, not `condition = "{{classify.kind}} == bug"`.

## Delivery contract

When asked to create or optimize a formula:

1. Prefer writing/updating a complete TOML file.
2. Include root fields: `formula`, `description`, `version`, `type = "workflow"`.
3. Use short stable local step ids: `fetch-pr`, `classify`, `run-tests`, `report`.
4. Make execution order explicit with `depends_on`.
5. Make data flow explicit with `input_context` for agent consumers.
6. Prefer deterministic `tool`, `aggregate`, or `script` steps before agent steps.
7. Use `[steps.validate]` for JSON that drives downstream structure, conditions, loops, or tools.
8. Validate with `tt formula validate`, `tt formula compile`, and, when safe, `tt formula run --dry-run`.

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
| Does it require judgment, tradeoff, synthesis, code reasoning, or prose? | agent step, omit `execution` |
| Is it repeat-until or foreach runtime iteration? | `[steps.loop]` |
| Is it stable reusable workflow reuse? | `embed = "child-formula"` |

### 3. Draw the data flow

Use step ids as data contracts:

```text
fetch-data -> aggregate-manifest -> write-files -> report
```

Rules:

- `depends_on` controls order.
- `input_context` injects prior outputs into agent prompts.
- Prefer whole step ids in `input_context`, not field paths.
- Do not feed huge content to an agent if a deterministic step can shrink or materialize it first.

### 4. Move deterministic work out of agents

Do **not** ask an agent to:

- create directories or write generated files when a tool/script can do it;
- run `git fetch`, `git checkout`, `git push`, or branch operations;
- sleep/wait a fixed duration;
- parse/filter JSON that `aggregate` can project;
- run tests or CLI commands;
- validate that a file exists if a command/tool can check it.

Agent steps should receive curated facts and produce judgments/reports.

## Minimal formula skeleton

```toml
formula = "example-workflow"
description = "Short repeatable workflow description."
version = 1
type = "workflow"

[vars]
topic = { description = "Thing to process", required = true }

[[steps]]
id = "analyze"
title = "Analyze {{topic}}"
description = "Analyze the topic and output concise findings."

[steps.agent]
name = "planner"
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
- Use `product-manager` for product requirements/tradeoffs.
- Use `writer` or `reporter` for user-facing prose.
- If output drives `condition`, `loop.until`, `aggregate`, or tools, require compact JSON and validate it.
- Do not mix long Markdown and control JSON in one step. Split them.

### 2. Tool step, for built-in deterministic operations

Use ToolStep for common computer actions. Current built-ins:

- `write_files`: create directory and write many files from JSON objects.
- `sleep`: wait a fixed duration.
- `git_fetch`: run `git fetch`.
- `git_push`: run `git push`.
- `git_branch`: list/create/delete branches.
- `git_checkout`: checkout or create branch.
- `git_worktree`: create/list/remove git worktrees for isolated feature development.

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

### 8. Embed step, for stable reusable child workflows

```toml
[[steps]]
id = "bug-fix"
title = "Run bug-fix workflow"
embed = "bug-fix"

[steps.embed_vars]
issue_summary = "{{triage.issue_summary}}"
```

Embed rules:

- Use embed for stable SOP reuse.
- Do not mix `embed` with loop, script, tool, agent, form, or children on the same step.
- Downstream dependencies wait for the embedded workflow exit boundary.

### 9. Noop step, only for real graph structure

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
3. Dependencies reference existing step ids.
4. Deterministic operations use `tool`, `aggregate`, or `script`, not agent prose.
5. Agent steps have clear goal, inputs, output format, and constraints.
6. JSON outputs that control downstream behavior have validation.
7. Large content is materialized or projected before reporting.
8. Human input uses `human_input` or `form = true`, not “ask the user” prose.
9. Loops have `max` and inspectable bodies.
10. Validation/compile/dry-run commands pass.
