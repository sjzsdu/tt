#!/bin/bash
set -u
wt="${TT_WORKTREE_PATH:-}"
base="${TT_BASE_BRANCH:-main}"
if [ -z "$wt" ] || [ ! -d "$wt" ]; then
  jq -cn --arg error "worktree path missing" '{attempted:false,success:false,conflict:false,error:$error,conflict_files:[]}'
  exit 0
fi
cd "$wt"
if ! git diff --quiet || ! git diff --cached --quiet; then
  jq -cn '{attempted:false,success:false,conflict:false,error:"worktree has uncommitted changes before rebase",conflict_files:[]}'
  exit 0
fi
git fetch origin "$base" --prune >/dev/null 2>&1
out="$(git rebase "origin/$base" 2>&1)"
rc=$?
if [ "$rc" -eq 0 ]; then
  jq -cn --arg output "$out" --arg head "$(git rev-parse HEAD)" '{attempted:true,success:true,conflict:false,in_rebase:false,needs_resolution:false,error:"",stdout:$output,head:$head,conflict_files:[]}'
  exit 0
fi
files="$(git diff --name-only --diff-filter=U | jq -R . | jq -s .)"
in_rebase=false
if [ -d "$(git rev-parse --git-path rebase-merge 2>/dev/null)" ] || [ -d "$(git rev-parse --git-path rebase-apply 2>/dev/null)" ]; then
  in_rebase=true
fi
jq -cn --arg output "$out" --argjson conflict_files "$files" --argjson in_rebase "$in_rebase" '{attempted:true,success:false,conflict:($conflict_files|length>0),in_rebase:$in_rebase,needs_resolution:(($conflict_files|length>0) or $in_rebase),error:$output,stdout:$output,conflict_files:$conflict_files}'
