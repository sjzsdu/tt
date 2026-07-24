# Team TOML Schema Reference

## Top-level fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `team` | string | yes | — | Unique team name (slug, lowercase) |
| `title` | string | no | same as `team` | Human-readable display name |
| `description` | string | no | — | Brief description of the team's purpose |
| `version` | int | yes | 1 | Schema version (always 1) |

## `[coordination]`

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `facilitator` | string | no | first agent id | Agent id that coordinates the discussion |
| `finalizer` | string | no | same as facilitator | Agent id that produces the final answer |
| `review_waves` | int | no | 0 | Number of baseline broad peer-review windows; directed adaptive turns may run in addition |
| `max_concurrency` | int | no | 4 | Max parallel agent slots |

## `[limits]`

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `max_agent_turns` | int | no | 24 | Hard cap on initial, adaptively activated, and finalization turns in a round |
| `max_wall_time` | string | no | "15m" | Max wall-clock time per round (e.g. "5m", "30m", "1h") |

## `[memory]`

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `enabled` | bool | no | true | Whether team memory is maintained |
| `maintainer` | string | no | same as finalizer | Agent id responsible for memory updates |
| `path` | string | no | — | Custom memory file path |
| `max_chars` | int | no | 20000 | Max memory size (must be >= 1000) |

## `[[agents]]` (array, at least 2 required)

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `id` | string | yes | — | Unique agent identifier |
| `role` | string | no | — | Short role description |
| `agent` | string | no | "assistant" | Agent type: `assistant`, `planner`, or `coder` |
| `model` | string | no | — | Model override for this agent |
| `prompt` | string | no | — | System prompt / instructions for this agent |
| `can_finalize` | bool | no | false | Whether this agent can produce the final answer |

## Validation rules

1. `team` must be non-empty.
2. `version` must be >= 1.
3. At least 2 `[[agents]]` entries.
4. All agent `id` values must be unique (case-insensitive).
5. `coordination.facilitator`, `coordination.finalizer`, and `memory.maintainer` must reference a valid agent `id`.
6. `coordination.max_concurrency` must be >= 1.
7. `limits.max_agent_turns` must be >= `len(agents) + review_waves * (len(agents) - 1) + 1`.
8. `memory.max_chars` must be >= 1000.
