#!/bin/bash
# 准备合并分支
set -euo pipefail

json_error() {
  local msg="$1"
  echo "{\"ok\":false,\"error\":\"$msg\",\"items\":[]}"
  exit 1
}

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

branches_raw="${TT_BRANCHES:-}"
target_input="${TT_TARGET_BRANCH:-}"
base_input="${TT_BASE_BRANCH:-}"

if [ -z "${branches_raw//[[:space:],]/}" ]; then
  json_error "branches is required"
fi

# 检查是否有 git 操作进行中
if [ -f "$(git rev-parse --git-path MERGE_HEAD)" ] || \
   [ -d "$(git rev-parse --git-path rebase-merge)" ] || \
   [ -d "$(git rev-parse --git-path rebase-apply)" ] || \
   [ -f "$(git rev-parse --git-path CHERRY_PICK_HEAD)" ]; then
  json_error "git operation already in progress"
fi

# 检查工作区是否干净
if ! git diff --quiet || ! git diff --cached --quiet; then
  json_error "working tree or index is not clean"
fi

current_branch="$(git branch --show-current)"
base_branch="$current_branch"
base_ref="$current_branch"

if [ -n "$base_input" ] && [ "$base_input" != "-" ]; then
  base_ref="$base_input"
  base_branch="$base_input"
fi

if [ -z "$base_ref" ]; then
  base_ref="HEAD"
  base_branch="$(git rev-parse --short HEAD)"
fi

if ! git rev-parse --verify --quiet "$base_ref^{commit}" >/dev/null; then
  json_error "base_branch cannot be resolved: $base_ref"
fi

base_head="$(git rev-parse "$base_ref^{commit}")"

target="$target_input"
if [ -z "$target" ] || [ "$target" = "-" ]; then
  target="tmp-merge-$(date +%Y%m%d-%H%M%S)"
fi

case "$target" in
  tmp*) ;;
  *) target="tmp-$target" ;;
esac

if git show-ref --verify --quiet "refs/heads/$target"; then
  json_error "target branch already exists: $target"
fi

items_json="$(python3 - <<'PY' "$branches_raw"
import json, sys
raw = sys.argv[1]
seen = set()
items = []
for part in raw.split(','):
    branch = part.strip()
    if not branch or branch in seen:
        continue
    seen.add(branch)
    items.append({"branch": branch, "index": len(items) + 1})
print(json.dumps(items, ensure_ascii=False))
PY
)"

if [ "$(jq 'length' <<<"$items_json")" = "0" ]; then
  json_error "no valid branches parsed from comma-separated input"
fi

missing="$(jq -r '.[].branch' <<<"$items_json" | while IFS= read -r b; do git rev-parse --verify --quiet "$b^{commit}" >/dev/null || printf '%s\n' "$b"; done | jq -R -s -c 'split("\n") | map(select(length>0))')"

if [ "$(jq 'length' <<<"$missing")" != "0" ]; then
  echo "{\"ok\":false,\"error\":\"one or more branches cannot be resolved\",\"missing_branches\":$missing,\"items\":[]}"
  exit 1
fi

git switch -c "$target" "$base_ref" >/dev/null 2>&1

jq -cn \
  --arg repo_root "$repo_root" \
  --arg base_branch "$base_branch" \
  --arg base_ref "$base_ref" \
  --arg base_head "$base_head" \
  --arg current_branch "$current_branch" \
  --arg target_branch "$target" \
  --arg created_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --argjson items "$items_json" \
  '{ok:true,repo_root:$repo_root,current_branch:$current_branch,base_branch:$base_branch,base_ref:$base_ref,base_head:$base_head,target_branch:$target_branch,created_at:$created_at,items:$items,total:($items|length)}'
