#!/bin/bash
set -u
meta="${TT_PR_META:-{}}"
repo_hint="${TT_REPO_HINT:-}"
base_override="${TT_BASE_BRANCH:-}"
root_input="${TT_WORKTREE_ROOT:-.tt/worktrees/github-pr-rebase-main}"
current_check="${TT_CURRENT_CHECK:-{}}"
ok="$(jq -r '.ok // false' <<<"$meta")"
if [ "$ok" != true ]; then
  jq -cn --arg error "pr metadata unavailable" --arg metadata_raw "$meta" '{ok:false,error:$error,metadata_raw:$metadata_raw,worktree_path:"",base_branch:"",head_branch:""}'
  exit 0
fi
number="$(jq -r '.number // empty' <<<"$meta")"
head="$(jq -r '.headRefName // empty' <<<"$meta")"
base="$(jq -r '.baseRefName // empty' <<<"$meta")"
if [ -n "$base_override" ] && [ "$base_override" != "-" ]; then base="$base_override"; fi
if [ -z "$number" ] || [ -z "$head" ] || [ -z "$base" ]; then
  jq -cn --arg error "missing PR number/head/base" --arg metadata_raw "$meta" '{ok:false,error:$error,metadata_raw:$metadata_raw,worktree_path:"",base_branch:"",head_branch:""}'
  exit 0
fi
invocation_dir="$(pwd)"
repo_root="$(git rev-parse --show-toplevel 2>/dev/null)"
if [ -z "$repo_root" ]; then
  jq -cn '{ok:false,error:"current directory is not inside a git repository",worktree_path:"",base_branch:"",head_branch:""}'
  exit 0
fi
if [ "$(jq -r '.current_on_pr_branch // false' <<<"$current_check")" = true ]; then
  worktree_path="$(jq -r '.worktree_path // empty' <<<"$current_check")"
  [ -n "$worktree_path" ] || worktree_path="$repo_root"
  safe_head="$(printf '%s' "$head" | tr -c 'A-Za-z0-9._-' '-')"
  worktree_root="$root_input"
  case "$worktree_root" in /*) ;; *) worktree_root="$invocation_dir/$worktree_root" ;; esac
  stale_target="$worktree_root/pr-${number}-${safe_head}"
  stale_target_real="$(cd "$stale_target" 2>/dev/null && pwd -P || printf '%s' "$stale_target")"
  worktree_path_real="$(cd "$worktree_path" 2>/dev/null && pwd -P || printf '%s' "$worktree_path")"
  if [ "$stale_target_real" = "$worktree_path_real" ]; then
    stale_target=""
  elif [ ! -d "$stale_target/.git" ] && [ ! -f "$stale_target/.git" ]; then
    stale_target=""
  fi
  cd "$worktree_path"
  git fetch origin "$base" --prune >/dev/null 2>&1
  current_branch="$(git branch --show-current)"
  upstream="$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null || true)"
  head_sha="$(git rev-parse HEAD)"
  base_sha="$(git rev-parse "origin/$base" 2>/dev/null || true)"
  jq -cn \
    --arg worktree_path "$worktree_path" \
    --arg worktree_root "" \
    --arg invocation_dir "$invocation_dir" \
    --arg repo_root "$repo_root" \
    --arg base_branch "$base" \
    --arg head_branch "$head" \
    --arg current_branch "$current_branch" \
    --arg upstream "$upstream" \
    --arg head_sha "$head_sha" \
    --arg base_sha "$base_sha" \
    --arg stale_target_worktree_path "$stale_target" \
    '{ok:true,error:"",worktree_path:$worktree_path,worktree_root:$worktree_root,invocation_dir:$invocation_dir,repo_root:$repo_root,base_branch:$base_branch,head_branch:$head_branch,current_branch:$current_branch,upstream:$upstream,head_sha:$head_sha,base_sha:$base_sha,checkout_output:"",reused:true,created:false,reused_current:true,stale_target_worktree_path:$stale_target_worktree_path}'
  exit 0
fi
cd "$repo_root"
safe_head="$(printf '%s' "$head" | tr -c 'A-Za-z0-9._-' '-')"
worktree_root="$root_input"
case "$worktree_root" in /*) ;; *) worktree_root="$invocation_dir/$worktree_root" ;; esac
worktree_path="$worktree_root/pr-${number}-${safe_head}"
local_branch="$head"
pr_head_ref="refs/tt/pr-rebase/${number}/head"
mkdir -p "$worktree_root"
reused=false
created=false
reused_current=false
fetch_out=""
if [ -d "$worktree_path/.git" ] || [ -f "$worktree_path/.git" ]; then
  if git -C "$worktree_path" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    reused=true
  else
    jq -cn --arg error "existing path is not a valid git worktree" --arg worktree_path "$worktree_path" --arg base_branch "$base" --arg head_branch "$head" '{ok:false,error:$error,worktree_path:$worktree_path,base_branch:$base_branch,head_branch:$head_branch,reused:false,created:false}'
    exit 0
  fi
else
  if ! fetch_out="$(git fetch origin "+refs/pull/${number}/head:${pr_head_ref}" "$base" --prune 2>&1)"; then
    jq -cn --arg error "$fetch_out" --arg worktree_path "$worktree_path" --arg base_branch "$base" --arg head_branch "$head" '{ok:false,error:$error,worktree_path:$worktree_path,base_branch:$base_branch,head_branch:$head_branch,reused:false,created:false}'
    exit 0
  fi
  git branch -D "$local_branch" >/dev/null 2>&1 || true
  if ! git worktree add -B "$local_branch" "$worktree_path" "$pr_head_ref" >/tmp/tt-pr-rebase-worktree.log 2>&1; then
    err="$(cat /tmp/tt-pr-rebase-worktree.log)"
    jq -cn --arg error "$err" --arg worktree_path "$worktree_path" --arg base_branch "$base" --arg head_branch "$head" '{ok:false,error:$error,worktree_path:$worktree_path,base_branch:$base_branch,head_branch:$head_branch,reused:false,created:false}'
    exit 0
  fi
  created=true
fi
cd "$worktree_path"
if [ "$reused" = true ]; then
  if ! git diff --quiet || ! git diff --cached --quiet; then
    jq -cn --arg error "existing target worktree has uncommitted changes before PR checkout" --arg worktree_path "$worktree_path" --arg base_branch "$base" --arg head_branch "$head" '{ok:false,error:$error,worktree_path:$worktree_path,base_branch:$base_branch,head_branch:$head_branch,reused:true,created:false}'
    exit 0
  fi
  if ! fetch_out="$(git -C "$repo_root" fetch origin "+refs/pull/${number}/head:${pr_head_ref}" "$base" --prune 2>&1)"; then
    jq -cn --arg error "$fetch_out" --arg worktree_path "$worktree_path" --arg base_branch "$base" --arg head_branch "$head" '{ok:false,error:$error,worktree_path:$worktree_path,base_branch:$base_branch,head_branch:$head_branch,reused:true,created:false}'
    exit 0
  fi
  current_branch_before="$(git branch --show-current 2>/dev/null || true)"
  if [ "$current_branch_before" != "$head" ]; then
    checkout_out="$(git checkout -B "$local_branch" "$pr_head_ref" 2>&1)"
    checkout_rc=$?
    if [ "$checkout_rc" -ne 0 ]; then
      jq -cn --arg error "$checkout_out" --arg worktree_path "$worktree_path" --arg base_branch "$base" --arg head_branch "$head" '{ok:false,error:$error,worktree_path:$worktree_path,base_branch:$base_branch,head_branch:$head_branch,reused:true,created:false}'
      exit 0
    fi
  else
    checkout_out="target worktree already on PR head branch; fetched ${pr_head_ref}"
  fi
else
  checkout_out="$fetch_out"
fi
current_branch="$(git branch --show-current)"
if [ "$current_branch" != "$head" ]; then
  jq -cn --arg error "target worktree is on unexpected branch: $current_branch" --arg worktree_path "$worktree_path" --arg base_branch "$base" --arg head_branch "$head" '{ok:false,error:$error,worktree_path:$worktree_path,base_branch:$base_branch,head_branch:$head_branch,reused:false,created:false}'
  exit 0
fi
git fetch origin "$base" --prune >/dev/null 2>&1
upstream="$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null || true)"
head_sha="$(git rev-parse HEAD)"
base_sha="$(git rev-parse "origin/$base" 2>/dev/null || true)"
jq -cn \
  --arg worktree_path "$worktree_path" \
  --arg worktree_root "$worktree_root" \
  --arg invocation_dir "$invocation_dir" \
  --arg repo_root "$repo_root" \
  --arg base_branch "$base" \
  --arg head_branch "$head" \
  --arg current_branch "$current_branch" \
  --arg upstream "$upstream" \
  --arg head_sha "$head_sha" \
  --arg base_sha "$base_sha" \
  --arg checkout_output "$checkout_out" \
  --argjson reused "$reused" \
  --argjson created "$created" \
  '{ok:true,error:"",worktree_path:$worktree_path,worktree_root:$worktree_root,invocation_dir:$invocation_dir,repo_root:$repo_root,base_branch:$base_branch,head_branch:$head_branch,current_branch:$current_branch,upstream:$upstream,head_sha:$head_sha,base_sha:$base_sha,checkout_output:$checkout_output,reused:$reused,created:$created,reused_current:false}'
