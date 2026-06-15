#!/bin/bash
set -u
wt="${TT_WORKTREE_PATH:-}"
repo_root="${TT_REPO_ROOT:-}"
reused_current="${TT_REUSED_CURRENT:-false}"
stale_target="${TT_STALE_TARGET_WORKTREE_PATH:-}"
rebase_blocked="${TT_REBASE_BLOCKED:-false}"
if [ -z "$wt" ]; then
  jq -cn '{attempted:false,removed:false,worktree_path:"",stdout:"",stderr:"missing worktree_path"}'
  exit 0
fi
if [ "$rebase_blocked" = true ]; then
  jq -cn --arg worktree_path "$wt" '{attempted:true,removed:false,skipped_reason:"rebase_blocked",worktree_path:$worktree_path,stdout:"",stderr:""}'
  exit 0
fi
if [ "$reused_current" = true ]; then
  if [ -n "$stale_target" ] && [ -e "$stale_target" ]; then
    stale_out="$(git -C "$repo_root" worktree remove --force --force "$stale_target" 2>&1)"
    stale_rc=$?
    git -C "$repo_root" worktree prune >/dev/null 2>&1 || true
    jq -cn --arg worktree_path "$wt" --arg stale_target_worktree_path "$stale_target" --arg output "$stale_out" --argjson stale_removed "$([ "$stale_rc" -eq 0 ] && echo true || echo false)" '{attempted:true,removed:false,skipped_reason:"reused_current_workspace",worktree_path:$worktree_path,stale_target_worktree_path:$stale_target_worktree_path,stale_target_removed:$stale_removed,stdout:(if $stale_removed then $output else "" end),stderr:(if $stale_removed then "" else $output end)}'
    exit 0
  fi
  jq -cn --arg worktree_path "$wt" '{attempted:true,removed:false,skipped_reason:"reused_current_workspace",worktree_path:$worktree_path,stdout:"",stderr:""}'
  exit 0
fi
if [ -z "$repo_root" ] || [ ! -d "$repo_root/.git" ]; then
  repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi
if [ ! -e "$wt" ]; then
  git -C "$repo_root" worktree prune >/dev/null 2>&1 || true
  jq -cn --arg worktree_path "$wt" '{attempted:true,removed:true,already_absent:true,worktree_path:$worktree_path,stdout:"",stderr:""}'
  exit 0
fi
out="$(git -C "$repo_root" worktree remove --force --force "$wt" 2>&1)"
rc=$?
git -C "$repo_root" worktree prune >/dev/null 2>&1 || true
jq -cn --arg worktree_path "$wt" --arg output "$out" --argjson removed "$([ "$rc" -eq 0 ] && echo true || echo false)" '{attempted:true,removed:$removed,already_absent:false,worktree_path:$worktree_path,stdout:(if $removed then $output else "" end),stderr:(if $removed then "" else $output end)}'
