---
name: formula-writer
description: 'Design, write, refactor, validate, and troubleshoot tt formula TOML workflows for the typed runtime. Use when the user asks to create/optimize a formula, design a repeatable workflow, add script/agent/human-input steps, model runtime branches/loops, embed subflows, or debug formula compile/run behavior.'
license: MIT
---

# Formula Writer

Use this skill to author **new-architecture `tt formula` workflows**. A formula is not a big prompt. It is a typed, resumable, auditable workflow graph executed by the typed runtime.

A good formula separates:

- deterministic fact collection and validation: `execution = "script"`
- reasoning, synthesis, planning, implementation, review, reporting: agent steps
- user choices or private missing context: `execution = "human_input"` or dynamic `form = true`
- runtime routing and iteration: `input_context`, `condition`, `[steps.loop]` 和 step-id 输出
- stable reusable SOPs: `embed = "child-formula"`

## Current architecture assumptions

- `tt formula run` uses the typed runtime by default. Do not mention or rely on legacy engine flags.
- Formula command implementation lives under `cmd/formula`; UI model helpers live in `internal/formulaui`; saved-run view helpers live in `internal/formularunview`.
- Canonical formula file names are `<name>.toml`. Do not create `.formula.toml` files.
- Compiled output is a Workflow IR graph. Authors usually should not write manual boundary/noop steps unless they need real structure there.
- Step output is saved under the local step `id` by default. New formulas should reference prior outputs by step id.

## Delivery contract

When asked to create or optimize a formula, deliver complete TOML unless the user asks for another format.

The TOML must:

1. include root fields: `formula`, `description`, `version = 1`, `type = "workflow"`.
2. use project/user canonical path `.tt/formulas/<name>.toml` when writing files.
3. use stable short local step ids like `fetch-pr`, `classify`, `implement`, not compiled ids like `my-formula.fetch-pr`.
4. make dependencies explicit with `depends_on` or `needs`.
5. make data flow explicit with step ids and `input_context` where downstream logic consumes prior output.
6. use `execution = "script"` for deterministic local commands and safe argv `command = [...]`.
7. use agent steps for judgment, synthesis, implementation, and reporting.
8. use human-input steps instead of prompt text like “ask the user” when runtime user input is required.
9. validate compact JSON outputs that drive conditions or loops using `[steps.validate]`.
10. validate with `tt formula validate`, `tt formula compile`, and `tt formula run --dry-run` before claiming readiness.

## Minimal TOML skeleton

```toml
formula = "example-workflow"
description = "Short description of the repeatable workflow."
version = 1
type = "workflow"

[vars]
topic = { description = "Thing to process", required = true }

[[steps]]
id = "analyze"
title = "Analyze {{topic}}"
description = "Analyze the topic and output a concise result."

[steps.agent]
name = "planner"
```

## Variables

Use variables for user-provided run-time values:

```toml
[vars]
pr = { description = "Pull request number", required = true, pattern = "^\\d+$" }
env = { description = "Target environment", default = "staging", enum = ["staging", "prod"] }
dry_run = { description = "Whether to avoid writes", default = "true", type = "bool" }
```

Variables are rendered with `{{var}}` in titles, descriptions, script argv, env, and cwd.

`tt formula run <name> <value>` positional shorthand works only when exactly one variable is required.

## Step model

Common fields:

| Field | Purpose |
|---|---|
| `id` | Local stable step id. Unique in the formula. |
| `title` | Human-readable title. Supports `{{var}}`. |
| `description` | Instructions for agent steps or form/script context. |
| `depends_on` / `needs` | Step ids this step waits for. |
| `execution` | Omit for agent, or use `script`, `human_input`, `noop`. |
| step `id` | Runtime context key for this step output. Reference this id from `input_context`, `condition`, and `loop.until`. |
| `input_context` | Step ids whose complete JSON outputs are injected into the prompt. Prefer whole step ids instead of field-level paths. |
| `condition` | Runtime expression deciding whether the step runs. |
| `timeout` | Step timeout, for example `30s`, `5m`, `1h`. |
| `agent` | Agent config for agent steps. |
| `script` | Script config for script steps. |
| `form` | Static form for human input, or `form = true` for dynamic agent clarification. |
| `validate` | Runtime output validation, especially JSON shape. |
| `loop` | Runtime loop body and stop condition. |
| `embed` | Compile-time inline reusable child workflow. |

## Agent steps

Agent steps are default when `execution` is omitted.

Use embedded agents when possible:

- `planner`: decomposition, workflow design, strategy.
- `coder`: code analysis, implementation, debugging.
- `tester`: tests, validation, quality strategy.
- `product-manager`: requirements, tradeoffs, product judgment.
- `ui`: UI/UX tasks.
- `full-stack`: cross-cutting app work.
- `reporter`: final user-facing report.

For outputs that drive `condition`, `loop.until`, or structured downstream prompts, require compact JSON and validate it:

```toml
[[steps]]
id = "classify"
title = "Classify request"
description = """
Classify the request.
Output ONLY compact JSON:
{"kind":"frontend|backend|infra","confidence":0.0,"reason":"..."}
"""

[steps.agent]
name = "planner"

[steps.validate]
format = "json"
required = ["kind", "confidence", "reason"]
```

## Script steps

Use scripts for deterministic facts and validations. Prefer direct argv commands.

```toml
[[steps]]
id = "fetch-pr"
title = "Fetch PR metadata"
execution = "script"

[steps.script]
command = ["gh", "pr", "view", "{{pr}}", "--json", "number,title,body,files"]
format = "json"
timeout = "30s"
```

Script rules:

- Prefer `command = ["tool", "arg", "{{var}}"]`.
- Use `command = ["bash", "-lc", "set -euo pipefail; ..."]` only for short glue.
- Use Python only when jq/bash would be unclear or unsafe.
- Always set `timeout` for non-trivial commands.
- Use `continue_on_error = true` for diagnostic commands whose failures should become data.
- Do not generate destructive commands like `rm`, `sudo`, `chmod`, `chown`, `dd`, `mkfs`, shutdown/reboot, or pipe-to-shell patterns.
- `shell = "bash"` is explicit shell mode and requires `--allow-shell-script`; avoid it unless the user explicitly wants it.

Script output is saved as an envelope containing command, cwd, exit_code, stdout, stderr, json, and duration_ms.

## Human input

### Static human-input step

Use this when the workflow must pause for a known user decision.

```toml
[[steps]]
id = "choose-option"
title = "Choose implementation option"
execution = "human_input"

[steps.form]
title = "Choose an option"
description = "The workflow resumes after submission."
submit_label = "Continue"

[[steps.form.fields]]
name = "option"
label = "Selected option"
type = "radio"
required = true
options = ["safe", "fast", "complete"]
```

Supported field types: `input`, `textarea`, `radio`, `checkbox`, `select`. Choice fields require `options`.

Submit via CLI:

```bash
tt formula run input latest choose-option --field option=safe
```

### Dynamic human input from an agent

Use `form = true` on an agent step when the agent may need to ask for missing information only if necessary. Prefer this over static fields for triage/bug reports, because the agent can generate the smallest question set from the actual missing context.

```toml
[[steps]]
id = "triage"
title = "Triage and clarify if needed"
form = true
description = """
If required information is missing, dynamically clarify it.
Otherwise output ONLY compact JSON:
{"ready":true,"summary":"..."}
"""

[steps.agent]
name = "planner"

[steps.validate]
format = "json"
required = ["ready", "summary"]
```

The runtime injects the dynamic `tt-human-input` protocol into the agent prompt. If the agent emits a request, the run enters `waiting_input`, saves the form, and resumes after submission.

## Conditions

Conditions are runtime expressions over variables and saved outputs. Use bare names, not `{{...}}`.

Good:

```toml
condition = "classify.kind == frontend"
condition = "env == prod"
condition = "review.approved == true"
condition = "risk.score != high"
```

Bad:

```toml
condition = "{{env}} == prod"
```

Supported patterns include equality, inequality, regex match, JSON path lookup, `&&`, and `||`.

When using a JSON path condition, ensure the producing step outputs valid compact JSON and uses its step id as the context key. For `input_context`, normally reference the producing step id so the full JSON output is available downstream.

## Runtime loops

Use `[steps.loop]` when the number of iterations depends on runtime output or input data.

```toml
[[steps]]
id = "improve"
title = "Improve until approved"
depends_on = ["classify"]
condition = "classify.kind == frontend"
input_context = ["classify"]
description = "Iterate on a draft until review approves."

  [steps.loop]
  until = "review.approved == true"
  max = 3

  [[steps.loop.body]]
  id = "draft"
  title = "Draft iteration {{iteration}}"
  description = "Create or improve the draft."

  [[steps.loop.body]]
  id = "review"
  title = "Review iteration {{iteration}}"
  depends_on = ["draft"]
  input_context = ["draft"]
  description = "Output ONLY compact JSON: {\"approved\":true,\"reason\":\"...\"}."

  [steps.loop.body.validate]
  format = "json"
  required = ["approved", "reason"]
```

Loop guidance:

- Always set `max`.
- The body step referenced by `until` must have an id matching the key used by the condition.
- Use `{{iteration}}` in titles/descriptions.
- Body steps can be agent, script, or human_input.

## Embedding reusable workflows

Use `embed` to inline a stable child workflow at compile time.

```toml
[[steps]]
id = "security-review"
title = "Run security review"
embed = "security-checklist"

[steps.embed_vars]
service = "{{service}}"
level = "strict"
```

Rules:

- `embed` cannot mix with children, loop, agent, script, or form on the same step.
- Downstream dependencies on the embed step wait for the embedded workflow exit boundary.
- Use embed only for stable SOP reuse, not one-off decomposition.

## Recommended patterns

### Deterministic context -> agent analysis -> validation -> report

```toml
[[steps]]
id = "fetch-pr"
title = "Fetch PR metadata"
execution = "script"
[steps.script]
command = ["gh", "pr", "view", "{{pr}}", "--json", "number,title,body,files"]
format = "json"
timeout = "30s"

[[steps]]
id = "review"
title = "Review risks"
depends_on = ["fetch-pr"]
input_context = ["fetch-pr"]
description = "Output ONLY compact JSON: {\"risks\":[],\"missing_tests\":[],\"summary\":\"...\"}."
[steps.agent]
name = "coder"
[steps.validate]
format = "json"
required = ["risks", "missing_tests", "summary"]

[[steps]]
id = "run-tests"
title = "Run tests"
depends_on = ["review"]
execution = "script"
[steps.script]
command = ["go", "test", "./..."]
format = "text"
timeout = "5m"
continue_on_error = true

[[steps]]
id = "report"
title = "Write final report"
depends_on = ["run-tests"]
input_context = ["review", "run-tests"]
description = "Write a concise final report with risks, tests, and recommended next steps."
[steps.agent]
name = "reporter"
```

### Runtime decision branch

```toml
[[steps]]
id = "classify"
title = "Classify request"
description = "Output ONLY compact JSON: {\"path\":\"bug|feature\"}."
[steps.agent]
name = "planner"
[steps.validate]
format = "json"
required = ["path"]

[[steps]]
id = "bug-path"
title = "Bug fix path"
depends_on = ["classify"]
input_context = ["classify"]
condition = "classify.path == bug"

[[steps]]
id = "feature-path"
title = "Feature design path"
depends_on = ["classify"]
input_context = ["classify"]
condition = "classify.path == feature"
```

## Commands to use

Authoring and validation:

```bash
tt formula validate .tt/formulas/<name>.toml
tt formula compile <name> --dir .tt/formulas
tt formula run <name> --dir .tt/formulas --dry-run
```

Running and saved runs:

```bash
tt formula run <name> --var key=value
tt formula runs --formula <name>
tt formula run show latest
tt formula run show latest --step <step-id>
tt formula run open latest
tt formula run resume latest
tt formula run input latest <step-id> --field key=value
```

There is no legacy engine flag. Do not recommend `--runtime-engine` or `--legacy-engine`.

## Troubleshooting

### Parse or validate error

- Check TOML nesting. Each `[[steps]]` starts a new table.
- Use `[steps.agent]`, `[steps.script]`, `[steps.form]`, `[steps.validate]` under the current step.
- Loop body uses `[[steps.loop.body]]` and nested tables like `[steps.loop.body.agent]` or `[steps.loop.body.validate]`.
- Arrays must be TOML arrays: `depends_on = ["a", "b"]`.

### Compile error: missing dependency

- `depends_on` and `needs` reference local step ids in the same formula.
- Do not reference compiled ids such as `formula.step`.
- Usually do not reference generated start/end boundary ids.

### Condition does not work

- Use bare names, not `{{...}}`.
- Ensure producer output is valid JSON if using paths like `classify.path`.
- Ensure the consumer depends on the producer.
- Prefer clear, stable step ids for condition inputs.

### Loop never stops

- Ensure `until` references the id of a loop body step.
- Ensure that output is valid JSON.
- Always set `max`.

### Waiting for input

- Inspect: `tt formula run show latest --step <step-id>`.
- Submit: `tt formula run input latest <step-id> --field key=value`.
- Repeat the same `--field` key for checkbox/multi-value fields.
- Live dashboard can submit forms; historical dashboards are read-only.

## Final self-check before handing off

Before saying a formula is ready:

1. File name is `<name>.toml`, not `.formula.toml`.
2. Root fields are complete and `formula` matches the name.
3. Every step has unique local `id` and clear `title`.
4. Dependencies reference existing local ids.
5. Deterministic facts use script steps with safe argv commands and timeout.
6. Agent steps choose appropriate embedded agents.
7. Outputs consumed downstream have stable step ids and downstream `input_context` references the whole producing step ids.
8. JSON outputs used by conditions/loops have `[steps.validate]`.
9. Human input uses `execution = "human_input"` or `form = true`, not “ask the user” prose.
10. Loops have `max` and correct body step ids.
11. `tt formula validate`, `tt formula compile`, and `tt formula run --dry-run` pass.
