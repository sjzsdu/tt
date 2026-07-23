# Team Patterns

## Product review team

A cross-functional team for evaluating product requirements and technical feasibility.

```toml
team = "product-review"
title = "Product Review"
version = 1

[coordination]
facilitator = "facilitator"
finalizer = "facilitator"
review_waves = 1
max_concurrency = 3

[limits]
max_agent_turns = 8
max_wall_time = "15m"

[memory]
enabled = true
maintainer = "facilitator"
max_chars = 20000

[[agents]]
id = "facilitator"
role = "Facilitator and product lead"
agent = "assistant"
can_finalize = true
prompt = """
Clarify the goal, expose unresolved disagreements, and synthesize a decisive answer.
You are a normal team member, not a message broker or manager with special authority.
"""

[[agents]]
id = "architect"
role = "Software architect"
agent = "planner"
prompt = """
Focus on system boundaries, failure modes, evolution paths, and technical tradeoffs.
Challenge vague assumptions and propose concrete architecture.
"""

[[agents]]
id = "engineer"
role = "Implementation engineer"
agent = "coder"
prompt = """
Focus on implementation cost, compatibility, tests, operations, and incremental delivery.
Turn broad ideas into changes that can actually ship.
"""
```

## Bug triage team

Quick incident triage and root-cause analysis.

```toml
team = "bug-triage"
title = "Bug Triage"
version = 1

[coordination]
facilitator = "incident-lead"
finalizer = "incident-lead"
review_waves = 0
max_concurrency = 3

[limits]
max_agent_turns = 6
max_wall_time = "10m"

[memory]
enabled = false

[[agents]]
id = "incident-lead"
role = "Incident commander"
agent = "assistant"
can_finalize = true
prompt = """
Triage the bug report, identify severity and impact, and produce a clear action plan.
"""

[[agents]]
id = "debugger"
role = "Root cause analyst"
agent = "coder"
prompt = """
Focus on reproducing the issue, identifying root cause, and suggesting minimal fixes.
Check logs, stack traces, and recent changes.
"""

[[agents]]
id = "qa"
role = "Quality assurance"
agent = "assistant"
prompt = """
Verify the bug report is complete, identify edge cases, and suggest test scenarios.
"""
```

## Architecture review team

Deep technical review of architecture decisions and proposals.

```toml
team = "arch-review"
title = "Architecture Review"
version = 1

[coordination]
facilitator = "reviewer"
finalizer = "reviewer"
review_waves = 1
max_concurrency = 2

[limits]
max_agent_turns = 6
max_wall_time = "20m"

[memory]
enabled = true
maintainer = "reviewer"
max_chars = 15000

[[agents]]
id = "reviewer"
role = "Architecture reviewer"
agent = "planner"
can_finalize = true
prompt = """
Evaluate the proposal against scalability, maintainability, and cost tradeoffs.
Surface risks and propose alternatives.
"""

[[agents]]
id = "skeptic"
role = "Technical skeptic"
agent = "planner"
prompt = """
Challenge assumptions, identify failure modes, and stress-test the proposal.
Ask "what happens when X?" for critical X values.
"""
```

## Security audit team

Evaluate code or infrastructure for security concerns.

```toml
team = "security-audit"
title = "Security Audit"
version = 1

[coordination]
facilitator = "auditor"
finalizer = "auditor"
review_waves = 1
max_concurrency = 3

[limits]
max_agent_turns = 8
max_wall_time = "20m"

[memory]
enabled = true
maintainer = "auditor"
max_chars = 20000

[[agents]]
id = "auditor"
role = "Security lead"
agent = "assistant"
can_finalize = true
prompt = """
Coordinate the audit, classify findings by severity, and produce a prioritized report.
"""

[[agents]]
id = "code-reviewer"
role = "Code security reviewer"
agent = "coder"
prompt = """
Scan for OWASP Top 10, injection flaws, auth bypass, data exposure, and insecure dependencies.
"""

[[agents]]
id = "infra-reviewer"
role = "Infrastructure security reviewer"
agent = "planner"
prompt = """
Review network policies, IAM roles, secrets management, and container security.
"""
```
