---
name: team-writer
description: Design, write, refactor, and validate tt team TOML definitions. Use when creating or optimizing team files, choosing agent roles, tuning coordination/memory/limits configs, or modeling multi-agent collaboration patterns. Trigger on "写一个team", "设计team", "team definition", "team toml", "create team", "team config", "team schema", "team-writer", "帮我写个team", "优化team配置".
---

# Team Writer

Design, write, and validate `tt team` TOML definitions.

## Quick start

Minimal team (2 agents required):

```toml
team = "my-team"
title = "My Team"
version = 1

[coordination]
facilitator = "lead"
finalizer = "lead"
review_waves = 1
max_concurrency = 3

[limits]
max_agent_turns = 8
max_wall_time = "15m"

[memory]
enabled = true
maintainer = "lead"
max_chars = 20000

[[agents]]
id = "lead"
role = "Team lead"
agent = "assistant"
can_finalize = true
prompt = "Coordinate the team and synthesize a final answer."

[[agents]]
id = "coder"
role = "Implementation engineer"
agent = "coder"
prompt = "Focus on implementation, tests, and incremental delivery."
```

## Workflow

1. Understand the team's purpose (what problem it solves, what roles are needed).
2. Choose agent roles and assign `agent` types (`assistant`, `planner`, `coder`).
3. Write the TOML following the schema below.
4. Validate: `tt team validate <file>` or ensure the file passes `tt team list`.
5. Save to `.tt/teams/<name>.toml` (project) or `~/.tt/teams/<name>.toml` (global).

## Schema reference

Full field reference: see [references/schema.md](references/schema.md).

Key constraints:
- At least 2 agents required.
- `coordination.facilitator`, `coordination.finalizer`, `memory.maintainer` must reference a valid agent `id`.
- `max_agent_turns` must be >= `len(agents) + review_waves * (len(agents) - 1) + 1`.
- `memory.max_chars` must be >= 1000.
- Agent `id` values must be unique (case-insensitive).

## Agent types

Available `agent` values: `assistant`, `planner`, `coder`.

- `assistant` — general-purpose, good for facilitation, product, QA
- `planner` — analysis, architecture, design
- `coder` — implementation, debugging, ops

## Patterns

See [references/patterns.md](references/patterns.md) for common team patterns:
- Product review team
- Bug triage team
- Architecture review team
- Security audit team

## Coordination tips

- `review_waves = 0` — no baseline broad review; directed mentions and objections can still activate members
- `review_waves = 1` — schedule one broad peer-review window, plus any adaptive turns needed to resolve directed questions or objections
- `max_concurrency` — parallel agent slots; match to agent count for max parallelism
- `can_finalize = true` — only ONE agent should finalize (the facilitator)
- `max_agent_turns` — hard runtime budget including initial assessments, adaptive review turns, and finalization; increase it when roles are expected to challenge each other

The runtime appends collaboration-signal instructions automatically. Agent prompts should encourage natural, substantive discussion and explicit `@member-id` addressing rather than hard-coding orchestration steps.

## Memory

- `enabled = true` — team maintains durable memory across threads
- `maintainer` — agent responsible for updating memory (usually the facilitator)
- `max_chars` — 20000 is a good default; increase for complex domains
