#!/usr/bin/env bash
set -euo pipefail

level="${1:-compile}"

case "$level" in
  parse|compile)
    # Level 1: every user-facing builtin and every atomic builtin is loadable,
    # valid, and compilable with smoke variables. No external network or
    # side-effecting commands are executed.
    go test ./internal/formula -run 'TestAllBuiltinFormulasCompile|TestBuiltinAtomicFormulas' -count=1
    ;;
  formula)
    # Formula package plus formula command tests. Still no external e2e.
    go test ./internal/formula/... ./cmd/formula
    ;;
  all)
    "$0" compile
    "$0" formula
    ;;
  *)
    cat >&2 <<'USAGE'
Usage: scripts/validate-builtin-formulas.sh [compile|formula|all]

Levels:
  compile  Parse/validate/compile all builtin and atomic formulas with smoke vars.
  formula  Run formula package and command tests.
  all      Run both levels.
USAGE
    exit 2
    ;;
esac
