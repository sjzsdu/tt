#!/bin/bash
# 尝试合并分支
set -u

repo_root="${TT_REPO_ROOT:-}"
branch="${TT_BRANCH:-}"
index="${TT_INDEX:-}"

cd "$repo_root" 2>/dev/null || {
  echo "{\"branch\":\"$branch\",\"index\":$index,\"attempted\":false,\"merged\":false,\"conflict\":false,\"skipped\":true,\"error\":\"repo root missing\",\"conflict_files\":[]}"
  exit 0
}

before_head="$(git rev-parse HEAD 2>/dev/null || true)"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
out="$(git merge "$branch" 2>&1)"
rc=$?
after_head="$(git rev-parse HEAD 2>/dev/null || true)"
conflict_files="$(git diff --name-only --diff-filter=U | jq -R -s -c 'split("\n") | map(select(length>0))')"

if [ "$rc" -eq 0 ]; then
  jq -cn \
    --arg branch "$branch" \
    --arg index "$index" \
    --arg before_head "$before_head" \
    --arg after_head "$after_head" \
    --arg started_at "$started_at" \
    --arg completed_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg output "$out" \
    '{branch:$branch,index:($index|tonumber? // 0),attempted:true,merged:true,conflict:false,skipped:false,before_head:$before_head,after_head:$after_head,started_at:$started_at,completed_at:$completed_at,stdout:$output,error:"",conflict_files:[]}'
  exit 0
fi

if [ "$(jq 'length' <<<"$conflict_files")" != "0" ] || [ -f "$(git rev-parse --git-path MERGE_HEAD 2>/dev/null)" ]; then
  jq -cn \
    --arg branch "$branch" \
    --arg index "$index" \
    --arg before_head "$before_head" \
    --arg after_head "$after_head" \
    --arg started_at "$started_at" \
    --arg output "$out" \
    --argjson conflict_files "$conflict_files" \
    '{branch:$branch,index:($index|tonumber? // 0),attempted:true,merged:false,conflict:true,needs_resolution:true,skipped:false,before_head:$before_head,after_head:$after_head,started_at:$started_at,stdout:$output,error:$output,conflict_files:$conflict_files}'
  exit 0
fi

jq -cn \
  --arg branch "$branch" \
  --arg index "$index" \
  --arg before_head "$before_head" \
  --arg after_head "$after_head" \
  --arg started_at "$started_at" \
  --arg output "$out" \
  '{branch:$branch,index:($index|tonumber? // 0),attempted:true,merged:false,conflict:false,needs_resolution:false,skipped:true,before_head:$before_head,after_head:$after_head,started_at:$started_at,stdout:$output,error:$output,conflict_files:[]}'
