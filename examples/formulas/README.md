# Formula runtime control flow demos

This directory contains small formulas demonstrating Workflow IR runtime decisions, loops, deterministic script steps, and human input pauses.

`tt` also includes a small curated builtin formula for generating a multi-document learning pack about a new topic:

```bash
tt formula show fresh-topic-docs
tt formula run fresh-topic-docs "空间计算" --dry-run
tt formula copy fresh-topic-docs .tt/formulas/fresh-topic-docs.toml
```

## Demo formulas

- `runtime-control-demo.toml` demonstrates agent-driven runtime branching and loops.
- `script-pr-review-demo.toml` demonstrates script steps that collect deterministic PR/test context before handing it to agents.

## Runtime control flow

`runtime-control-demo.toml` demonstrates:

1. A step writes compact JSON under its `id`, for example `id = "decide"`.
2. Later steps use JSON-path conditions:
   - `condition = "decide.path == frontend"`
   - `condition = "decide.path == backend"`
3. A runtime `loop.until` repeats body steps until agent output satisfies:
   - `until = "review.approved == true"`

## Script steps

Use `execution = "script"` when a step should run a deterministic local command instead of an agent. Prefer direct argv-style commands because they avoid shell injection and are easier to audit. For light multi-command glue, prefer short bash argv scripts such as `command = ["bash", "-lc", "set -euo pipefail; ..."]`; use Python only when bash/CLI/jq is not enough for complex structured processing:

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

The saved output is a JSON envelope containing `command`, `cwd`, `exit_code`, `stdout`, `stderr`, parsed `json` when `format = "json"`, and `duration_ms`. Downstream steps can consume it with `input_context = ["fetch-pr"]` or conditions.

## Human input pauses

Use `execution = "human_input"` when a formula must stop at a known gate and collect structured user input before continuing. For triage-style missing context where the necessary questions are only known at runtime, prefer an agent step with `form = true`; the typed runtime injects the dynamic `tt-human-input` protocol and still owns pause/resume state.

```toml
[[steps]]
id = "choose-path"
title = "Choose path"
execution = "human_input"

[steps.form]
title = "Choose the next path"

[[steps.form.fields]]
name = "path"
label = "Path"
type = "radio"
required = true
options = ["frontend", "backend", "infra"]
```

When a run reaches this step, it enters `waiting_input`. Submit through the live dashboard modal or through the CLI:

```bash
tt formula run input latest choose-path --field path=frontend
```

Prefer static `execution = "human_input"` only for known gates. Prefer `form = true` for dynamic clarification. Do not invent ad-hoc protocols; use the built-in `tt-human-input` fenced JSON block that typed runtime parses and exposes to CLI/dashboard tooling.

Safety policy:

- Script execution is local to your machine and should be treated like running code from the formula file.
- Dangerous commands and patterns such as `rm`, `sudo`, `dd`, `mkfs`, and `rm -rf` are denied.
- `--no-script` disables script steps for a run.
- Shell mode is disabled by default. Prefer `command = [...]`; if you intentionally use `[steps.script] shell = "bash"`, run with `--allow-shell-script`.

## Validate without calling an LLM

```bash
tt formula compile runtime-control-demo --dir examples/formulas
tt formula run runtime-control-demo --dir examples/formulas --dry-run

tt formula compile script-pr-review-demo --dir examples/formulas --var pr=123
tt formula run script-pr-review-demo 123 --dir examples/formulas --dry-run
```

## Run with agents

```bash
tt formula run runtime-control-demo --dir examples/formulas --agent coder
tt formula run script-pr-review-demo 123 --dir examples/formulas --agent coder
```

Formula runs open the live web dashboard by default. Use `--no-web` only for automation or headless runs.

If the run enters `waiting_input`, keep the live dashboard open and submit the displayed form, or use `tt formula run input latest <step-id> --field key=value` from the terminal. Historical dashboards opened with `tt formula run open` are read-only.

After running, inspect persisted state:

```bash
tt formula runs --formula runtime-control-demo
tt formula run show latest
tt formula run open latest
```

## Expected runtime behavior

- The `decide` step should output JSON like `{"path":"frontend"}`.
- Only the matching branch step should execute.
- The `improve` loop runs its body until the `review` step outputs JSON like `{"approved":true}` or reaches `max = 3`.
- Script steps run without invoking agents and save their command result under the step id.
- Human input steps pause the run, save `human_input_request.json`, then resume after a response is submitted.
