---
name: formula-writer
description: 'Write, edit, validate, and troubleshoot tt formula templates (TOML/JSON) for structured agent/script/human-input workflows. Use when user asks to create a formula, edit a formula, design a workflow template, decompose tasks, add runtime branching/loops, use script steps, add human intervention forms, use curated builtin formulas, or debug formula compile/run issues.'
license: MIT
---

# Formula Writer

Use this skill to author and debug `tt formula` templates. A formula is a structured workflow that can mix:

- **agent steps** for reasoning, planning, coding, reviewing, and reporting.
- **script steps** for deterministic local commands such as `gh pr view`, `git diff`, `go test`, `jq`, or `curl`.
- **human input steps** for explicit user choices, confirmations, and missing private context.
- **runtime control flow** through `output_key`, `input_context`, `condition`, and `loop.until`.

The goal is to write formulas that are reusable, auditable, safe to run, and easy for future agents to debug.

## When to use formulas

Use a formula when the user wants a repeatable workflow, for example:

- PR review workflow: fetch PR metadata -> analyze risks -> run tests -> write report.
- Feature workflow: design -> implement -> test -> review -> summarize.
- Incident workflow: collect logs -> branch by symptom -> diagnose -> propose fix.
- Research workflow: gather deterministic context -> synthesize -> produce decision memo.
- Human-gated workflow: propose options -> collect user choice -> continue with the selected path.

Do **not** put every action into an agent step. If a step can be expressed as a deterministic command, prefer `execution = "script"` and pass its output to later agent steps.

Do **not** write prompts that say “ask the user” inside an autonomous formula. If the workflow needs user input during execution, use `execution = "human_input"` or instruct the agent to emit a dynamic `tt-human-input` block.

## Curated builtin formula

`tt` keeps only a small curated builtin set. Current builtin:

- `fresh-topic-docs`: generate a multi-document Markdown learning pack for a new thing, trend, concept, product, person, technology, or phenomenon.

Useful commands:

```bash
tt formula list --builtin
tt formula show fresh-topic-docs
tt formula run fresh-topic-docs "空间计算" --dry-run
tt formula copy fresh-topic-docs .tt/formulas/fresh-topic-docs.toml
```

## File location and naming

Formula files are usually TOML:

- Project-level: `.tt/formulas/<name>.toml`
- User-level: `~/.tt/formulas/<name>.toml`
- Examples: `examples/formulas/*.toml`
- Builtin curated formulas: `internal/formula/builtin/formulas/*.toml`

The `formula` field should match the intended name:

```toml
formula = "pr-review"
description = "Review a pull request with deterministic context collection."
version = 1
type = "workflow"
```

## Minimal root structure

```toml
formula = "<unique-name>"
description = "<what this workflow does>"
version = 1
type = "workflow"

[vars]
# variable definitions

[[steps]]
# step definitions
```

Supported formula types include:

- `workflow`: normal runnable workflow.
- `expansion`: macro-like reusable step expansion.
- `aspect`: cross-cutting transformation formula.

Most user-facing formulas should be `type = "workflow"`.

## Variables

Variables are passed with `--var key=value`, or for `tt formula run` a positional shorthand is available when there is exactly one required variable.

```toml
[vars]
pr = { description = "Pull request number", required = true }
env = { description = "Environment", default = "staging", enum = ["staging", "prod"] }
version = { description = "Semver", pattern = "^\\d+\\.\\d+\\.\\d+$" }
dry_run = { description = "Dry run flag", default = "true", type = "bool" }
```

Use variables with `{{var}}` in titles, descriptions, notes, and script command argv values:

```toml
title = "Review PR #{{pr}}"
command = ["gh", "pr", "view", "{{pr}}", "--json", "title,body"]
```

## Step model

Every step needs a stable `id` and a useful `title`.

```toml
[[steps]]
id = "analyze"
title = "Analyze implementation risk"
description = "Use the collected context to identify risk areas."
depends_on = ["fetch-context"]
input_context = ["context"]
output_key = "analysis"

[steps.agent]
name = "coder"
```

Important fields:

| Field | Meaning |
|---|---|
| `id` | Unique ID within the formula. Keep it short and stable. |
| `title` | Human-readable step title. |
| `description` | Instructions for agent steps. Also useful for documentation. |
| `depends_on` / `needs` | Dependency IDs this step waits for. |
| `condition` | Runtime condition, often based on previous `output_key` JSON. |
| `input_context` | Output keys to inject into the agent prompt. |
| `output_key` | Saves step output into runtime context. |
| `execution` | `script`, `human_input`, or `noop`; omit for normal agent execution. |
| `agent` | Agent configuration for agent steps. |
| `script` | Script configuration for script steps. |
| `form` | Form configuration for `human_input` steps. |
| `timeout` | Step-level timeout string such as `30s`, `5m`, `1h`. |

Compiled recipes always include start/end boundary steps. You usually do not need to write them manually.

## Agent steps

Agent steps are the default when `execution` is omitted. Use them for reasoning and language-heavy work.

```toml
[[steps]]
id = "review"
title = "Review PR risk"
depends_on = ["fetch-pr", "diff-stat"]
input_context = ["pr_metadata", "diff_stat"]
description = "Summarize risks, missing tests, and reviewer focus areas."
output_key = "review_summary"

[steps.agent]
name = "coder"
```

Recommended embedded agents:

- `coder`: code analysis, implementation, debugging.
- `planner`: decomposition and plans.
- `tester`: testing strategy and validation.
- `product-manager`: requirements and product judgment.
- `ui`: UI/UX tasks.
- `full-stack`: cross-cutting implementation.
- `reporter`: final report or user-facing summary.

Use `tt agent --list` to inspect available agents. If an agent is missing, formula run preflight will report configured and embedded agents.

## Script steps

Use script steps for deterministic local commands. They do not call an agent. Prefer direct argv commands for one tool call; use short bash argv scripts for light glue; reserve Python for cases where bash/CLI/jq cannot express the transformation cleanly.

```toml
[[steps]]
id = "fetch-pr"
title = "Fetch PR metadata"
execution = "script"
output_key = "pr_metadata"

[steps.script]
command = ["gh", "pr", "view", "{{pr}}", "--json", "number,title,body,files"]
format = "json"
timeout = "30s"
```

Script fields:

| Field | Meaning |
|---|---|
| `command` | Preferred argv-style command array. Each element supports `{{var}}`. |
| `format` | `json` or `text`. `json` parses stdout into the output envelope. |
| `timeout` | Script-specific timeout. Overrides step timeout. |
| `cwd` | Optional working directory. Supports variables. |
| `env` | Extra environment variables. Values support variables. |
| `continue_on_error` | If true, non-zero exit still completes and saves output. |
| `shell` | Explicit shell mode, disabled by default unless run with `--allow-shell-script`. |

Script output is saved as a JSON envelope:

```json
{
  "command": ["gh", "pr", "view", "123", "--json", "title,body"],
  "cwd": "/repo",
  "exit_code": 0,
  "stdout": "...",
  "stderr": "...",
  "json": {"title":"..."},
  "duration_ms": 812
}
```

Later steps should consume the `output_key` via `input_context` or conditions.

## Human input steps

Use human input when the workflow must pause for a real user decision, confirmation, or missing private context. The run enters `waiting_input`, saves a form request, and resumes after the user submits the response through the live dashboard or CLI.

### Static form step

```toml
[[steps]]
id = "confirm-scope"
title = "Confirm scope"
execution = "human_input"
output_key = "scope_confirmation"

[steps.form]
title = "Confirm project scope"
description = "The workflow will continue after this form is submitted."

[[steps.form.fields]]
name = "scope"
label = "Scope"
type = "select"
required = true
options = ["small", "medium", "large"]

[[steps.form.fields]]
name = "notes"
label = "Additional notes"
type = "textarea"
required = false
```

Supported field types:

- `input`
- `textarea`
- `radio`
- `checkbox`
- `select`

`radio`, `checkbox`, and `select` fields must declare `options`.

Submit from CLI:

```bash
tt formula run input latest confirm-scope --field scope=medium --field notes="Prefer a pragmatic plan"
```

For checkbox fields, repeat the same key:

```bash
tt formula run input latest choose-tools --field tools=search --field tools=browser
```

### Dynamic human input from an agent

If an agent discovers at runtime that continuing would require guessing, instruct it to emit a fenced `tt-human-input` JSON block:

````markdown
```tt-human-input
{
  "reason": "Cannot choose a travel style without the user's budget preference.",
  "form": {
    "title": "Budget preference",
    "fields": [
      {"name":"budget","label":"Budget level","type":"radio","required":true,"options":["low","medium","high"]}
    ]
  }
}
```
````

The submitted response becomes the step output. Give the step an `output_key` and make downstream steps consume it with `input_context`.

Live dashboard runs show the form as a modal and resume automatically after submission. Historical dashboards opened with `tt formula run open` are read-only, so use the CLI input command for saved runs.

### Script safety rules

Treat formula files as executable workflow definitions.

- Prefer direct argv `command = [...]` for a single command such as `gh`, `git`, `go test`, `npm test`, `jq`, or `curl`.
- For short multi-command glue, prefer bash argv, for example `command = ["bash", "-lc", "set -euo pipefail; gh pr view ..."]`. Keep bash scripts small and auditable.
- Do not wrap simple `gh/git/jq/curl` calls in `python3 -c`; use Python only for complex JSON/text transformations that are awkward or unsafe in bash/jq.
- Avoid formula `shell = "bash"` mode unless explicitly needed; `command = ["bash", "-lc", "..."]` remains an argv command.
- Dangerous commands and patterns are denied, including `rm`, `rmdir`, `sudo`, `su`, `chmod`, `chown`, `dd`, `mkfs`, `shutdown`, `reboot`, `rm -rf`, and pipe-to-shell patterns.
- Users can disable scripts for a run with `tt formula run ... --no-script`.
- Shell mode requires explicit opt-in: `tt formula run ... --allow-shell-script`.
- Use `--dry-run` before running formulas that contain scripts.

## Runtime output and conditions

Use `output_key` to save step output. Later steps can use it through `input_context` or `condition`.

```toml
[[steps]]
id = "decide"
title = "Decide path"
description = "Output ONLY compact JSON: {\"path\":\"frontend\"} or {\"path\":\"backend\"}."
output_key = "decision"

[[steps]]
id = "frontend-plan"
title = "Frontend branch"
depends_on = ["decide"]
input_context = ["decision"]
condition = "decision.path == frontend"
```

Condition syntax:

- `env == prod`
- `env != prod`
- `name =~ ^api-`
- `decision.path == frontend`
- `review.approved == true`
- `cond1 && cond2`
- `cond1 || cond2`

Important: conditions use bare variable names, not `{{var}}`.

```toml
condition = "env == prod"      # correct
condition = "{{env}} == prod"  # wrong
```

## Runtime loops

Use `loop.until` when the loop stop condition depends on the latest runtime output.

```toml
[[steps]]
id = "improve"
title = "Improve until approved"
condition = "decision.path == frontend"

  [steps.loop]
  until = "review.approved == true"
  max = 3

  [[steps.loop.body]]
  id = "draft"
  title = "Draft iteration {{iteration}}"
  output_key = "draft"

  [[steps.loop.body]]
  id = "review"
  title = "Review iteration {{iteration}}"
  input_context = ["draft"]
  description = "Output ONLY compact JSON: {\"approved\":true} or {\"approved\":false}."
  output_key = "review"
```

Loop body steps may be agent or script steps. Use `{{iteration}}` in loop body title/description.

## Recommended workflow patterns

### Pattern: deterministic context -> agent analysis -> deterministic validation -> report

```toml
[[steps]]
id = "fetch-pr"
title = "Fetch PR metadata"
execution = "script"
output_key = "pr_metadata"
[steps.script]
command = ["gh", "pr", "view", "{{pr}}", "--json", "title,body,files"]
format = "json"

[[steps]]
id = "review"
title = "Review risk"
depends_on = ["fetch-pr"]
input_context = ["pr_metadata"]
output_key = "review_summary"
[steps.agent]
name = "coder"

[[steps]]
id = "test"
title = "Run tests"
depends_on = ["review"]
execution = "script"
output_key = "test_result"
[steps.script]
command = ["go", "test", "./..."]
format = "text"
continue_on_error = true

[[steps]]
id = "report"
title = "Write final report"
depends_on = ["test"]
input_context = ["review_summary", "test_result"]
[steps.agent]
name = "reporter"
```

### Pattern: runtime decision branch

```toml
[[steps]]
id = "classify"
title = "Classify issue"
description = "Output JSON: {\"kind\":\"bug\"} or {\"kind\":\"feature\"}."
output_key = "classification"

[[steps]]
id = "bug-path"
title = "Bug fix path"
depends_on = ["classify"]
condition = "classification.kind == bug"

[[steps]]
id = "feature-path"
title = "Feature design path"
depends_on = ["classify"]
condition = "classification.kind == feature"
```

### Pattern: human-gated decision

```toml
[[steps]]
id = "compare-options"
title = "Compare options"
description = "Output concise Markdown with 2-3 viable options and tradeoffs."
output_key = "options"

[[steps]]
id = "choose-option"
title = "Choose option"
depends_on = ["compare-options"]
input_context = ["options"]
execution = "human_input"
output_key = "chosen_option"

[steps.form]
title = "Choose an option"

[[steps.form.fields]]
name = "option"
label = "Selected option"
type = "input"
required = true

[[steps]]
id = "make-plan"
title = "Make execution plan"
depends_on = ["choose-option"]
input_context = ["options", "chosen_option"]
```

## Validation commands

Always validate formulas before telling the user they are ready.

```bash
tt formula validate .tt/formulas/<name>.toml
tt formula compile <name> --dir .tt/formulas --var key=value
tt formula run <name> --dir .tt/formulas --dry-run --var key=value
```

For examples:

```bash
tt formula compile script-pr-review-demo --dir examples/formulas --var pr=123
tt formula run script-pr-review-demo 123 --dir examples/formulas --dry-run
```

After a real run:

```bash
tt formula runs --formula <name>
tt formula run show latest
tt formula run show latest --step <step-id>
tt formula run open latest
tt formula run input latest <step-id> --field key=value
```

## Troubleshooting playbook

### Parse or validate error

1. Check TOML syntax: table nesting, quote escaping, and multiline strings.
2. Ensure root fields exist: `formula`, `version`, `type`.
3. Ensure step tables are `[[steps]]`; nested loop body uses `[[steps.loop.body]]`.
4. Check arrays use TOML syntax: `depends_on = ["a", "b"]`.

### Compile error: missing dependency

- Every `depends_on` / `needs` ID must match a step `id` in the same formula.
- Reference local step IDs, not compiled IDs. Use `depends_on = ["fetch-pr"]`, not `my-formula.fetch-pr`.
- For runtime start/end, usually do not reference generated boundary IDs.

### Run error: missing variable

- Add a default or pass `--var key=value`.
- For `tt formula run`, positional shorthand only works when there is exactly one required var.

### Run error: agent preflight failed

- Check `[steps.agent] name = "..."`.
- Prefer embedded IDs like `coder`, `planner`, `tester`, `reporter`.
- Run `tt agent --list`.
- Script steps should set `execution = "script"` so they do not require an agent.

### Script step denied by safety policy

- Replace dangerous command with a safer read-only command.
- Avoid `rm`, `sudo`, `chmod`, `dd`, `mkfs`, pipe-to-shell, and shell mode.
- If shell is absolutely required, use `shell = "bash"` and tell the user to run with `--allow-shell-script`, but prefer argv command instead.
- Use `--dry-run` to inspect script steps before execution.

### Script output is not valid JSON

- If `format = "json"`, stdout must be valid JSON and only JSON.
- Remove logging from stdout, redirect logs to stderr, or use `format = "text"`.
- For commands like `gh`, use `--json ...`.

### Condition does not behave as expected

- Use bare names: `decision.path == frontend`, not `{{decision.path}}`.
- Make the producing step output compact JSON if using JSON paths.
- Confirm the producing step has `output_key`.
- Confirm the consuming step has a dependency on the producing step.

### Loop never stops

- Ensure the loop body step writes the key used by `loop.until`.
- Ensure the output is valid JSON if using JSON path conditions.
- Always set a reasonable `max`.

### Run is waiting for input

- Inspect the waiting step: `tt formula run show latest --step <step-id>`.
- Submit values: `tt formula run input latest <step-id> --field key=value`.
- For checkbox fields, repeat `--field name=value`.
- If using dashboard, submit from the live run dashboard. Historical dashboards are read-only.

### Resume or saved run debugging

Use saved run commands:

```bash
tt formula runs --formula <name>
tt formula run show latest
tt formula run show latest --step <compiled-step-id>
tt formula run resume latest
```

Step artifacts are saved under `.tt/runs/formula/<formula>/<run-id>/steps/`.

Human input artifacts include `<step>.human_input_request.json`, `<step>.human_input_response.json`, and `<step>.output.md`.

## Authoring checklist

Before finishing a formula:

1. Root has `formula`, `description`, `version`, `type = "workflow"`.
2. Variables are documented and required/defaults are intentional.
3. Every step has a unique stable `id` and clear `title`.
4. Dependencies reference existing local step IDs.
5. Agent steps use appropriate embedded agents.
6. Script steps use argv `command = [...]`, timeout, and safe read-only commands where possible.
7. Human input steps are used instead of “ask the user” wording when the workflow needs runtime user input.
8. Human input forms have clear labels, required fields, and options for choice fields.
9. Steps that feed later logic have `output_key`.
10. Steps that consume prior output have both `depends_on` and `input_context`.
11. Conditions use bare names and JSON paths correctly.
12. Runtime loops have `max` and body outputs matching `until`.
13. Run `tt formula validate`, `compile`, and `run --dry-run`.
14. If scripts are present, document what local tools are required, such as `gh`, `git`, `go`, or `jq`.
