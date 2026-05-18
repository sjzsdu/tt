# Formula runtime control flow demo

This directory contains a small formula demonstrating runtime decisions and loops driven by agent output.

## Demo formula

- `runtime-control-demo.toml`

It demonstrates:

1. A step writes compact JSON to `output_key = "decision"`.
2. Later steps use JSON-path conditions:
   - `condition = "decision.path == frontend"`
   - `condition = "decision.path == backend"`
3. A runtime `loop.until` repeats body steps until agent output satisfies:
   - `until = "review.approved == true"`

## Validate without calling an LLM

```bash
tt formula compile runtime-control-demo --dir examples/formulas
tt formula run runtime-control-demo --dir examples/formulas --dry-run
```

## Run with agents

```bash
tt formula run runtime-control-demo --dir examples/formulas --agent coder --web
```

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
