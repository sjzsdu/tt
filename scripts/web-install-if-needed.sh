#!/usr/bin/env bash
set -euo pipefail

marker="web/node_modules/.package-lock.json"
needs_install=0

if [[ ! -d web/node_modules || ! -f "$marker" ]]; then
  needs_install=1
elif [[ web/package.json -nt "$marker" || web/package-lock.json -nt "$marker" ]]; then
  needs_install=1
fi

if [[ "$needs_install" -eq 0 ]]; then
  echo "web-install: up to date"
  exit 0
fi

echo "web-install: npm install"
(cd web && npm install)
