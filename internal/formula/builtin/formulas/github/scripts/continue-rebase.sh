#!/bin/bash
set -u
wt="${TT_WORKTREE_PATH:-}"
cd "$wt" 2>/dev/null || { jq -cn '{attempted:false,success:false,error:"worktree path missing",conflict_files:[]}'; exit 0; }
remaining="$(git diff --name-only --diff-filter=U | jq -R . | jq -s .)"
if [ "$(jq 'length' <<<"$remaining")" != "0" ]; then
  jq -cn --argjson conflict_files "$remaining" '{attempted:false,success:false,error:"unresolved conflict files remain",conflict_files:$conflict_files}'
  exit 0
fi
in_rebase=false
if [ -d "$(git rev-parse --git-path rebase-merge 2>/dev/null)" ] || [ -d "$(git rev-parse --git-path rebase-apply 2>/dev/null)" ]; then
  in_rebase=true
fi
if [ "$in_rebase" != true ]; then
  jq -cn --arg head "$(git rev-parse HEAD)" '{attempted:false,success:true,error:"",stdout:"rebase already completed by conflict resolver",head:$head,conflict_files:[]}'
  exit 0
fi
out="$(GIT_EDITOR=true git rebase --continue 2>&1)"
rc=$?
if [ "$rc" -eq 0 ]; then
  jq -cn --arg output "$out" --arg head "$(git rev-parse HEAD)" '{attempted:true,success:true,error:"",stdout:$output,head:$head,conflict_files:[]}'
  exit 0
fi
files="$(git diff --name-only --diff-filter=U | jq -R . | jq -s .)"
jq -cn --arg output "$out" --argjson conflict_files "$files" '{attempted:true,success:false,error:$output,stdout:$output,conflict_files:$conflict_files}'
