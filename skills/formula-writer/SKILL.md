---
name: formula-writer
description: 'Write and edit tt formula templates (TOML/JSON) for structured task decomposition. Use when user asks to create a formula, edit a formula, write a workflow template, or mentions "formula", "workflow template", "task decomposition template".'
license: MIT
---

# Formula Writer

Write tt formula templates — structured task definitions with variables, dependencies, and control flow.

## File Location

- Project-level: `.tt/formulas/<name>.toml`
- User-level: `~/.tt/formulas/<name>.toml`
- Filename becomes the formula name (without extension)

## Root Structure

```toml
formula = "<unique-name>"          # Required: unique identifier
description = "<what it does>"     # Optional: description
version = 1                        # Required: >= 1
type = "workflow"                  # Required: "workflow" or "expansion"
extends = ["parent-formula"]       # Optional: inherit from other formulas

[vars]                             # Optional: variable definitions
# ...

[[steps]]                          # Required: task steps
# ...

[compose]                          # Optional: control flow rules
# ...
```

## Variable Definitions

```toml
[vars]
# Shorthand: default value
env = "staging"

# Full definition
name = { description = "Feature name", required = true }
priority = { description = "Priority", default = "2" }
region = { description = "Region", enum = ["us-east", "cn-shanghai"] }
version = { description = "Version", pattern = "^\\d+\\.\\d+\\.\\d+$" }
```

### VarDef fields
| Field | Type | Description |
|-------|------|-------------|
| `description` | string | What this variable is for |
| `default` | string | Default value |
| `required` | bool | Must be provided |
| `enum` | string[] | Allowed values |
| `pattern` | string | Regex validation |
| `type` | string | `string`, `int`, `bool` |

## Step Definition

```toml
[[steps]]
id = "build"                    # Required: unique ID within formula
title = "Build the project"     # Required: task title
description = "Run go build"    # Description
type = "task"                   # task, bug, feature, epic, chore
priority = 1                    # 0-4, 0=highest
tags = ["backend"]              # Labels (TOML: tags, JSON: labels)
assignee = "alice"              # Default assignee
timeout = "30m"                 # Max duration

# Dependencies (serial execution)
depends_on = ["design"]         # Wait for "design" step
needs = ["design"]              # Alias for depends_on

# Conditional execution
condition = "env == prod"       # Only execute if condition matches

# Retry on failure
retry.max_attempts = 3
retry.on_exhausted = "fail"

# Async gate
gate.type = "manual"
gate.timeout = "2h"

# Nested children (epic hierarchies)
  [[steps.children]]
  id = "sub-task"
  title = "Sub task"
```

### Condition Expression Syntax

- `var == value` — equals
- `var != value` — not equals
- `var =~ regex` — regex match
- `cond1 && cond2` — AND
- `cond1 || cond2` — OR

**Important**: In conditions, use bare variable names (no `{{}}`):
```toml
condition = "env == prod && region =~ ^cn-"   # ✅ correct
condition = "{{env}} == prod"                  # ❌ wrong
```

## Control Flow: compose

### Parallel Branches

```toml
[compose]
[[compose.branch]]
from = "start"               # Fork from this step
steps = ["frontend", "backend"]  # Steps to run in parallel
join = "merge"               # Rejoin at this step
```

Rules:
- `from`, `steps`, `join` are all required
- All referenced step IDs must exist in `[[steps]]`
- Compilation auto-sets: parallel steps need `from`, `join` needs all parallel steps

Multiple branches:
```toml
[[compose.branch]]
from = "design"
steps = ["frontend", "backend", "docs"]
join = "integration"
```

### Gate Rules

```toml
[compose]
[[compose.gate]]
before = "deploy-prod"
condition = "all_tests_passed == true"
```

### Loop (in-step iteration)

```toml
[[steps]]
id = "test-each"
title = "Run tests"

# Fixed count
loop.count = 3
loop.body = [
  { id = "unit", title = "Unit tests" },
  { id = "integration", title = "Integration tests" }
]

# Range iteration
loop.range = "1..5"
loop.var = "i"
loop.body = [
  { id = "deploy", title = "Deploy region {i}" }
]

# Until condition
loop.until = "tests_passed == true"
loop.max = 5
loop.body = [
  { id = "run-tests", title = "Run test suite" }
]
```

### Expand & Map

```toml
[compose]
# Expand: apply to one specific step
[[compose.expand]]
target = "testing"
with = "test-suite"
vars = { framework = "go" }

# Map: apply to all matching steps (glob pattern)
[[compose.map]]
select = "test-*"
with = "test-suite"
```

## Variable Substitution

Use `{{variable}}` in titles, descriptions, notes:

```toml
[[steps]]
id = "deploy"
title = "Deploy {{feature_name}} to {{env}}"
description = "Deploy the {{feature_name}} feature to {{env}} environment"
```

Variables are preserved during compilation and substituted at instantiation:
```bash
tt formula instantiate my-formula --var feature_name="Auth" --var env=prod
```

## Inheritance

```toml
# child.toml
formula = "child"
extends = ["parent"]
version = 1
type = "workflow"

[[steps]]
id = "extra"
title = "Extra step"
depends_on = ["setup"]    # References parent's step
```

## Validation Checklist

When writing a formula, verify:
1. `formula`, `version`, `type` are set at root level
2. Every step has unique `id` and `title`
3. `depends_on` / `needs` reference existing step IDs
4. `condition` uses bare variable names (no `{{}}`)
5. `compose.branch` references only exist in `steps`
6. `loop.body` is non-empty when `loop` is defined
7. File is in `.tt/formulas/` or `~/.tt/formulas/`

## CLI Commands

```bash
tt formula list                              # List all formulas
tt formula show <name>                       # Show formula details
tt formula compile <name> --var key=value    # Compile to recipe
tt formula instantiate <name> -o json|text|prompt  # Instantiate tasks
tt formula validate <file>                   # Validate syntax
```
