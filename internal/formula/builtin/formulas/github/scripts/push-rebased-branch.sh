#!/bin/bash
set -u
push_flag="${TT_PUSH:-true}"
wt="${TT_WORKTREE_PATH:-}"
head_branch="${TT_HEAD_BRANCH:-}"
validation="${TT_VALIDATION:-}"
validation_success="$(jq -r 'if length==0 then true else (.success // false) end' <<<"$validation")"
if [ "$push_flag" != true ]; then
  jq -cn '{requested:false,pushed:false,skipped_reason:"push=false",stdout:"",stderr:""}'
  exit 0
fi
if [ "$validation_success" != true ]; then
  jq -cn --arg stderr "validation failed; push skipped" '{requested:true,pushed:false,skipped_reason:"validation_failed",stdout:"",stderr:$stderr}'
  exit 0
fi
cd "$wt" 2>/dev/null || { jq -cn '{requested:true,pushed:false,skipped_reason:"",stdout:"",stderr:"worktree path missing"}'; exit 0; }
upstream="$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null || true)"
remote="origin"
remote_branch="$head_branch"
if [ -n "$upstream" ] && printf '%s' "$upstream" | grep -q '/'; then
  remote="${upstream%%/*}"
  remote_branch="${upstream#*/}"
fi
[ -n "$remote_branch" ] || remote_branch="$(git branch --show-current)"
out="$(git push --no-verify --force-with-lease "$remote" "HEAD:$remote_branch" 2>&1)"
rc=$?
jq -cn \
  --arg remote "$remote" \
  --arg remote_branch "$remote_branch" \
  --arg output "$out" \
  --arg head "$(git rev-parse HEAD)" \
  --argjson pushed "$([ "$rc" -eq 0 ] && echo true || echo false)" \
  '{requested:true,pushed:$pushed,remote:$remote,remote_branch:$remote_branch,head:$head,stdout:(if $pushed then $output else "" end),stderr:(if $pushed then "" else $output end),skipped_reason:""}'
