#!/usr/bin/env bash
set -euo pipefail

# Fast validation for embedded atomic formulas.
# This intentionally avoids gh/network side effects and only checks that atomic
# formulas are hidden from the regular catalog, loadable by name, valid, and
# compilable as embedded building blocks.

go test ./internal/formula -run 'TestBuiltinAtomicFormulas' -count=1
