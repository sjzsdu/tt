#!/bin/bash
# 清理临时评论修复 worktree
set -u

workspace="${TT_WORKSPACE_PATH:-}"
repo_root="${TT_REPO_ROOT:-}"

if [ -z "$workspace" ]; then
  jq -cn '{attempted:false,removed:false,workspace_path:"",stderr:"missing workspace_path"}'
  exit 0
fi

if [ -z "$repo_root" ] || [ ! -d "$repo_root/.git" ]; then
  repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi

if [ ! -e "$workspace" ]; then
  git -C "$repo_root" worktree prune >/dev/null 2>&1 || true
  jq -cn --arg workspace_path "$workspace" '{attempted:true,removed:true,already_absent:true,workspace_path:$workspace_path,stdout:"",stderr:""}'
  exit 0
fi

out="$(git -C "$repo_root" worktree remove --force --force "$workspace" 2>&1)"
rc=$?
git -C "$repo_root" worktree prune >/dev/null 2>&1 || true

jq -cn --arg workspace_path "$workspace" --arg output "$out" --argjson removed "$([ "$rc" -eq 0 ] && echo true || echo false)" '{attempted:true,removed:$removed,already_absent:false,workspace_path:$workspace_path,stdout:(if $removed then $output else "" end),stderr:(if $removed then "" else $output end)}'
