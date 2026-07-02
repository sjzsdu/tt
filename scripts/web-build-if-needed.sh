#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <app> <npm-script>" >&2
  exit 2
fi

app="$1"
script="$2"
src="web/apps/${app}"
embedded="internal/webui/${app}/dist"
marker="${embedded}/index.html"

if [[ ! -d "$src" ]]; then
  echo "web app not found: $src" >&2
  exit 1
fi

needs_build=0
if [[ ! -f "$marker" ]]; then
  needs_build=1
elif find "$src" -path "$src/dist" -prune -o -newer "$marker" -print -quit | grep -q .; then
  needs_build=1
elif find web -maxdepth 1 \( -name package.json -o -name package-lock.json \) -newer "$marker" -print -quit | grep -q .; then
  needs_build=1
fi

if [[ "$needs_build" -eq 0 ]]; then
  echo "web-build:${app}: up to date"
  exit 0
fi

echo "web-build:${app}: building"
(cd web && npm run "$script")
rm -rf "$embedded"
mkdir -p "$(dirname "$embedded")"
cp -R "${src}/dist" "$embedded"
