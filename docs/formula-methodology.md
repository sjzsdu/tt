# Formula Authoring Methodology

Writing an excellent `tt formula` is less like writing a prompt and more like designing a repeatable operating procedure. A formula should make the workflow explicit: what facts are collected, where judgment happens, how decisions branch, when the run pauses for user input, and what final artifact the user receives.

## Current architecture

Formula now compiles directly into a graph-first typed `Workflow`. Runtime nodes hold concrete `steps.Step` implementations, such as agent, script, human input, noop, loop, retry, and future step kinds. There is no separate legacy task tree compile model in the formula execution path.

```text
Formula TOML -> Resolve/Expand/Filter -> Workflow IR -> typed runtime -> run store/dashboard
```

The runtime persists:

```text
.tt/runs/formula/<formula>/<run-id>/
  run.json
  workflow.json
  state.json
  logs.jsonl
  steps/<step>.prompt.md
  steps/<step>.output.md
  steps/<step>.error.txt
```

## Core principle

Use deterministic steps for facts and validation, and agent steps for judgment and synthesis.

```text
script collects facts -> agent reasons -> script validates -> agent reports
```

If the workflow needs a user decision or missing information that the agent should not guess, pause explicitly through human input. Prefer dynamic clarification (`form = true`) when the need for clarification depends on runtime context.

## Formula design process

### 1. Start with a natural-language SOP

Before writing TOML, describe the workflow as a short operating procedure.

Example for PR review:

```text
Input: PR number.
Fetch PR metadata from GitHub.
Fetch diff statistics from git.
Ask a coder agent to analyze risk.
Run tests.
Ask a reporter agent to summarize findings and test result.
```

If the SOP is unclear, the formula will be unclear.

### 2. Classify each step

| Question | Use |
|---|---|
| Can a command/API return the answer? | `execution = "script"` |
| Does it require judgment, tradeoff, or explanation? | agent step, omit `execution` |
| Does it need a known user choice at this point? | `execution = "human_input"` |
| Might the agent need clarification only sometimes? | agent step with `form = true` |
| Does it only structure the graph? | `execution = "noop"` |

### 3. Draw the data flow using step ids

Step outputs are saved under the step `id` by default. Do **not** add `output_key` for normal authoring. Choose clear stable ids and reference those ids from downstream steps.

```text
fetch-pr -> review-risk -> run-tests -> report
```

Any step that consumes prior output should declare both:

- `depends_on = [...]` for execution order.
- `input_context = [...]` for data injected into the prompt.

Example:

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
id = "review-risk"
title = "Review implementation risk"
depends_on = ["fetch-pr"]
input_context = ["fetch-pr"]
description = "Review the PR metadata and output concise risk analysis."

[steps.agent]
name = "coder"
```

### 4. Add runtime control only where it buys value

Use `condition` when the workflow truly branches at runtime.

```toml
condition = "classify.kind == frontend"
```

Use `loop.until` only when an iteration can improve based on feedback.

```toml
[steps.loop]
until = "review.approved == true"
max = 3
```

Do not overuse branches and loops. A simple linear workflow is often better.

### 5. Validate the workflow as a product

A good formula should answer:

1. What input does the user provide?
2. What local tools does it require?
3. Which facts are collected deterministically?
4. Which steps require agent judgment?
5. What data flows between steps?
6. What happens if a command fails?
7. What is the final artifact?
8. How can a user debug a failed run?

## Agent step guidance

Agent steps are the default when `execution` is omitted. Their descriptions should include:

- Goal.
- Input context to use.
- Output format.
- Constraints.
- What not to invent.
- Success criteria.

For outputs that drive `condition` or `loop.until`, ask for compact JSON and validate it.

```toml
[[steps]]
id = "classify"
title = "Classify the issue"
description = """
Classify the issue.
Output ONLY compact JSON:
{"kind":"frontend|backend|infra","confidence":0.0,"reason":"..."}
"""

[steps.agent]
name = "planner"

[steps.validate]
format = "json"
required = ["kind", "confidence", "reason"]
```

Avoid mixing long Markdown and branch-control JSON in one output. Split the step if needed.

## Script step guidance

Use script steps to gather facts, call tools, and validate outcomes. Prefer argv commands:

```toml
[[steps]]
id = "run-tests"
title = "Run Go tests"
execution = "script"

[steps.script]
command = ["go", "test", "./..."]
format = "text"
timeout = "5m"
continue_on_error = true
```

Use `continue_on_error = true` when failure is information the report should include, such as tests failing. Leave it false when the workflow cannot continue without the result.

Script output is saved as an envelope with command, exit code, stdout, stderr, parsed JSON, and duration. Downstream agents should consume it with `input_context = ["run-tests"]`.

## Human input guidance

### Prefer dynamic clarification for uncertain missing context

Use `form = true` on an agent step when the agent should decide whether it needs user clarification. This keeps the formula concise and avoids static predeclared fields that may not be needed.

```toml
[[steps]]
id = "analyze-issue"
title = "Analyze issue and clarify if needed"
form = true
description = """
Analyze the issue. If information is missing and guessing would be unsafe,
dynamically clarify it. Otherwise output ONLY compact JSON:
{"summary":"...","assumptions":[],"next_action":"..."}
"""

[steps.agent]
name = "coder"

[steps.validate]
format = "json"
required = ["summary", "assumptions", "next_action"]
```

The runtime injects the `tt-human-input` protocol into the agent prompt. If the agent emits a request, the run enters `waiting_input`. The submitted response becomes the step output, keyed by the same step id.

### Use static human-input only for known gates

Use `execution = "human_input"` only when the formula author already knows the workflow must pause at that exact point, for example approval, option selection, or required private context.

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

Supported field types are `input`, `textarea`, `radio`, `checkbox`, and `select`. Choice fields require `options`.

Submit from the CLI:

```bash
tt formula run input latest choose-option --field option=safe
```

Live dashboard runs display the form and resume automatically after submission.

## Common workflow shapes

### Linear pipeline

```text
collect -> analyze -> validate -> report
```

### Fan-out / fan-in

```text
collect
  -> analyze-backend
  -> analyze-frontend
  -> analyze-tests
merge-report
```

### Runtime branch

```text
classify
  -> frontend-path if classify.kind == frontend
  -> backend-path if classify.kind == backend
  -> infra-path if classify.kind == infra
```

### Review loop

```text
draft -> review -> revise -> review ... until review.approved == true
```

### Human-gated workflow

```text
collect context -> propose options -> human_input -> branch or synthesize -> final report
```

## Formula canvas

```text
Formula name:
Goal:
User inputs:
Required local tools:
Final artifact:

Workflow shape:
Steps:
  - id:
    kind: agent/script/human_input/noop
    purpose:
    depends_on:
    input_context:
    condition:
    validation:
    failure behavior:
    success criteria:

Runtime branches:
Loops:
Human input:
Validation commands:
Safety concerns:
Debugging notes:
```

Ask the model to design this canvas first, then emit TOML.

## Quality checklist

A formula is ready when:

- The workflow can be explained in one paragraph.
- Every step has one clear responsibility.
- Facts and validations are script steps where practical.
- Agent steps have precise descriptions and output expectations.
- Step ids are stable and used as context keys.
- No normal step uses `output_key`.
- Dynamic clarification uses `form = true` when the question set should be runtime-generated.
- Static human-input steps are only used for known gates.
- Any branch/loop controller outputs compact JSON and has `[steps.validate]`.
- Dependencies describe the real execution order.
- Script steps have timeouts and safe commands.
- `tt formula validate` succeeds.
- `tt formula compile` shows the expected Workflow.
- `tt formula run --dry-run` is understandable.
- The final report step gives the user an actionable result.

## Debugging mindset

When a formula fails, debug the workflow like a data pipeline:

1. Did the producing step run?
2. Is the expected step-id key present in the runtime context?
3. Is the output valid JSON if a condition uses JSON paths?
4. Does the consuming step depend on the producer?
5. Is `input_context` present?
6. Did a script command fail or timeout?
7. Did agent preflight fail because an agent ID is wrong?
8. Would splitting a large step make the failure easier to isolate?

The best formulas are not the most complex. They are the ones whose behavior remains obvious after a failure.
