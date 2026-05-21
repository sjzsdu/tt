# Formula Authoring Methodology

Writing an excellent `tt formula` is less like writing a prompt and more like designing a repeatable operating procedure. A formula should make the workflow explicit: what facts are collected, where judgment happens, how decisions branch, how validation closes the loop, and what final artifact the user receives.

## Core principle

Use deterministic steps for facts and validation, and agent steps for judgment and synthesis.

```text
script collects facts -> agent reasons -> script validates -> agent reports
```

This keeps the workflow auditable and reduces hallucination. If a command can reliably fetch data, run it as a script step. If a human would need to weigh tradeoffs, explain risks, design a solution, or produce a report, use an agent step. For script implementation, prefer direct argv commands first, short bash argv scripts for light glue, and Python only when bash/CLI/jq cannot express the required structured processing cleanly.

If the workflow needs a user decision or missing information that the agent should not guess, use a human input pause instead of writing “ask the user” into an agent prompt. Formula execution is an autonomous workflow; a step that needs user input must explicitly suspend the run and resume after the user submits structured data.

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

Mark each step as one of:

- `script`: deterministic local command or tool call.
- `agent`: reasoning, synthesis, writing, decision-making.
- `human_input`: suspend the run and collect structured user input.
- `noop`: boundary, grouping, or structural coordination.

A useful rule:

| Question | Use |
|---|---|
| Can a command/API return the answer? | `script` |
| Does it require judgment, tradeoff, or explanation? | `agent` |
| Does it need a user choice, confirmation, or missing private context? | `human_input` |
| Does it only structure the graph? | `noop` |

### 3. Draw the data flow

For every useful output, name an `output_key`.

```text
fetch-pr -> pr_metadata
fetch-diff -> diff_stat
review -> review_summary
test -> test_result
report -> final_report
```

Any step that consumes prior output should declare both:

- `depends_on = [...]`
- `input_context = [...]`

Dependencies control execution order. Input context controls what the agent sees.

### 4. Add runtime control only where it buys value

Use `condition` when the workflow truly branches at runtime.

```toml
condition = "classification.kind == frontend"
```

Use `loop.until` only when an iteration can improve based on feedback.

```toml
until = "review.approved == true"
max = 3
```

Do not overuse branches and loops. A simple linear workflow is often better.

Use `human_input` when pausing is more reliable than guessing. Good candidates:

- Selecting one option before branching.
- Confirming constraints, budget, target audience, or risk tolerance.
- Uploading or pasting source material that is not available to the agent.
- Approving a draft before expensive follow-up work.

Avoid human input for questions the formula can answer from variables, local files, command output, or prior step outputs.

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

## Step quality rubric

A strong step has a single responsibility.

### Good step

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

Why it is good:

- It is deterministic.
- Its output is named.
- It has a timeout.
- It can be debugged independently.

### Weak step

```toml
[[steps]]
id = "do-everything"
title = "Review the PR and run tests and summarize everything"
```

Problems:

- Too many responsibilities.
- No explicit data flow.
- Hard to resume or debug.
- Forces the agent to guess facts it could have collected deterministically.

## Agent step guidance

Agent step descriptions should include:

- Goal.
- Input context to use.
- Output format.
- Constraints.
- What not to invent.
- Success criteria.

Example:

```toml
[[steps]]
id = "review-risk"
title = "Review implementation risk"
depends_on = ["fetch-pr", "diff-stat"]
input_context = ["pr_metadata", "diff_stat"]
output_key = "risk_review"
description = """
Review the PR using pr_metadata and diff_stat.

Focus on:
1. correctness risks
2. missing tests
3. migration or compatibility risks
4. suspicious large changes

Output concise Markdown with:
- Summary
- Risk areas
- Suggested tests
- Review checklist

Do not invent files, CI results, or runtime behavior not present in input context.
"""

[steps.agent]
name = "coder"
```

If an agent output controls `condition` or `loop.until`, ask it to output only compact JSON.

```toml
description = """
Classify the issue.
Output ONLY compact JSON:
{"kind":"frontend|backend|infra","confidence":0.0,"reason":"..."}
"""
output_key = "classification"
```

Avoid mixing long Markdown and branch-control JSON in one output. Split the step if needed.

## Script step guidance

Use script steps to gather facts, call tools, and validate outcomes.

Good script step candidates:

- `gh pr view ... --json ...`
- `git diff --stat ...`
- `go test ./...`
- `npm test`
- `kubectl get ... -o json`
- `terraform plan -no-color`
- `curl ...`

Prefer argv commands:

```toml
[steps.script]
command = ["go", "test", "./..."]
format = "text"
timeout = "2m"
continue_on_error = true
```

Use `continue_on_error = true` when failure is information the report should include, such as tests failing. Leave it false when the workflow cannot continue without the result.

Script output is saved as an envelope with command, exit code, stdout, stderr, parsed JSON, and duration. Downstream agents should consume the output with `input_context`.

## Human input guidance

There are two ways to pause for user input.

### Static form step

Use this when the formula author already knows the workflow requires user input at a specific point.

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

Supported field types are `input`, `textarea`, `radio`, `checkbox`, and `select`. `radio`, `checkbox`, and `select` fields must declare `options`.

### Dynamic clarification from an agent

Use this when an agent can decide at runtime that it is unsafe to continue without clarification. The agent should emit a fenced block named `tt-human-input`:

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

When this happens, the run enters `waiting_input`. The submitted response becomes the step output, so downstream steps should reference the step's `output_key` through `input_context`.

Submit from the CLI:

```bash
tt formula run input latest confirm-scope --field scope=medium --field notes="Prefer a pragmatic plan"
```

For checkbox fields, repeat the same key:

```bash
tt formula run input latest choose-tools --field tools=search --field tools=browser
```

Live dashboard runs display the form in a modal and resume automatically after submission.

## Common workflow shapes

### Linear pipeline

Best for straightforward SOPs.

```text
collect -> analyze -> validate -> report
```

### Fan-out / fan-in

Best when independent analyses can run after shared context collection.

```text
collect
  -> analyze-backend
  -> analyze-frontend
  -> analyze-tests
merge-report
```

### Runtime branch

Best when an early classifier chooses a path.

```text
classify
  -> frontend-path if classification.kind == frontend
  -> backend-path if classification.kind == backend
  -> infra-path if classification.kind == infra
```

### Review loop

Best for draft/review/revise workflows.

```text
draft -> review -> revise -> review ... until approved
```

### Validation sandwich

Best for implementation and debugging workflows.

```text
agent proposes -> script validates -> agent fixes/explains -> report
```

### Human-gated workflow

Best when the workflow needs a real user decision before continuing.

```text
collect context -> propose options -> human_input -> branch or synthesize -> final report
```

## Formula canvas

Before writing TOML, fill this out:

```text
Formula name:
Goal:
User inputs:
Required local tools:
Final artifact:

Workflow shape:
Steps:
  - id:
    type: agent/script/noop
    purpose:
    input_context:
    output_key:
    depends_on:
    failure behavior:
    success criteria:

Runtime branches:
Loops:
Validation commands:
Safety concerns:
Debugging notes:
```

This canvas is especially useful for LLMs. Ask the model to design the canvas first, then emit TOML.

## Practical examples to build experience

Start with three formulas and iterate on them:

1. **PR review**
   - `fetch-pr(script)`
   - `fetch-diff(script)`
   - `analyze-risk(agent)`
   - `run-tests(script)`
   - `report(agent)`

2. **Bug investigation**
   - `collect-error(script or user input)`
   - `classify(agent JSON)`
   - branch by frontend/backend/infra
   - `hypothesize(agent)`
   - `validate(script)`
   - `report(agent)`

3. **Feature implementation**
   - `understand-request(agent)`
   - `inspect-codebase(script/agent)`
   - `design(agent)`
   - `implement(agent)`
   - `test(script)`
   - `fix-loop(agent + script)`
   - `summarize(agent)`

4. **Decision workflow with human gate**
   - `collect-context(agent/script)`
   - `compare-options(agent JSON)`
   - `choose-option(human_input)`
   - `make-plan(agent)`
   - `report(agent)`

Each iteration should record what was hard to debug. Use those findings to improve step boundaries, output keys, and validation steps.

## Quality checklist

A formula is ready when:

- The workflow can be explained in one paragraph.
- Every step has one clear responsibility.
- Facts and validations are script steps where practical.
- Agent steps have precise descriptions and output expectations.
- Human input steps are used instead of “ask the user” wording inside autonomous agent prompts.
- Human input forms have clear labels, required fields, and options for choice fields.
- Any branch/loop controller outputs compact JSON.
- Every consumed output has an `output_key` and matching `input_context`.
- Dependencies describe the real execution order.
- Script steps have timeouts and safe commands.
- `tt formula compile` succeeds.
- `tt formula run --dry-run` is understandable.
- The final report step gives the user an actionable result.

## Debugging mindset

When a formula fails, debug the workflow like a data pipeline:

1. Did the producing step run?
2. Did it save the expected `output_key`?
3. Is the output valid JSON if a condition uses JSON paths?
4. Does the consuming step depend on the producer?
5. Is `input_context` present?
6. Did a script command fail or timeout?
7. Did agent preflight fail because an agent ID is wrong?
8. Would splitting a large step make the failure easier to isolate?

The best formulas are not the most complex. They are the ones whose behavior remains obvious after a failure.
