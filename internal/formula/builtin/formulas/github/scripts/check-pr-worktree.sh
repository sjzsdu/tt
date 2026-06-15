#!/bin/bash
set -u
meta="${TT_PR_META:-{}}"
root_input="${TT_WORKTREE_ROOT:-.tt/worktrees/github-pr-rebase-main}"
ok="$(jq -r '.ok // false' <<<"$meta")"
if [ "$ok" != true ]; then
  jq -cn --arg error "pr metadata unavailable" --arg metadata_raw "$meta" '{ok:false,already_on_pr_branch:false,error:$error,metadata_raw:$metadata_raw,current_branch:"",head_branch:"",worktree_path:""}'
  exit 0
fi
head="$(jq -r '.headRefName // empty' <<<"$meta")"
number="$(jq -r '.number // empty' <<<"$meta")"
if [ -z "$head" ]; then
  jq -cn --arg error "missing PR head branch" '{ok:false,already_on_pr_branch:false,error:$error,current_branch:"",head_branch:"",worktree_path:""}'
  exit 0
fi
if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  jq -cn --arg head_branch "$head" '{ok:true,already_on_pr_branch:false,error:"current directory is not inside a git repository",current_branch:"",head_branch:$head_branch,worktree_path:""}'
  exit 0
fi
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
repo_root_real="$(cd "$repo_root" && pwd -P)"
current_branch="$(git branch --show-current 2>/dev/null || true)"
invocation_dir="$(pwd)"
safe_head="$(printf '%s' "$head" | tr -c 'A-Za-z0-9._-' '-')"
expected_root="$root_input"
case "$expected_root" in /*) ;; *) expected_root="$invocation_dir/$expected_root" ;; esac
expected_path="$expected_root/pr-${number}-${safe_head}"
expected_path_real="$(cd "$expected_path" 2>/dev/null && pwd -P || printf '%s' "$expected_path")"
match_path=""
match_head=""
current_path=""
current_head=""
while IFS= read -r line; do
  case "$line" in
    worktree' '*) current_path="${line#worktree }" ;;
    HEAD' '*) current_head="${line#HEAD }" ;;
    branch' 'refs/heads/*)
      branch="${line#branch refs/heads/}"
      if [ "$branch" = "$head" ]; then
        match_path="$current_path"
        match_head="$current_head"
      fi
      ;;
    "")
      if [ -n "$match_path" ]; then break; fi
      current_path=""
      current_head=""
      ;;
  esac
done < <(git -C "$repo_root" worktree list --porcelain)
if [ -n "$match_path" ]; then
  match_path_real="$(cd "$match_path" 2>/dev/null && pwd -P || printf '%s' "$match_path")"
  if [ "$match_path_real" = "$repo_root_real" ]; then
    jq -cn \
      --arg current_branch "$head" \
      --arg head_branch "$head" \
      --arg worktree_path "$repo_root" \
      --arg head_sha "$match_head" \
      '{ok:true,already_on_pr_branch:false,current_on_pr_branch:true,error:"",reason:"current workspace is already on the PR head branch",current_branch:$current_branch,head_branch:$head_branch,worktree_path:$worktree_path,head_sha:$head_sha}'
    exit 0
  fi
  if [ "$match_path_real" = "$expected_path_real" ]; then
    jq -cn \
      --arg current_branch "$current_branch" \
      --arg head_branch "$head" \
      --arg worktree_path "$match_path" \
      --arg head_sha "$match_head" \
      '{ok:true,already_on_pr_branch:false,current_on_pr_branch:false,target_worktree_on_pr_branch:true,error:"",reason:"formula target worktree is already on the PR head branch and will be reused",current_branch:$current_branch,head_branch:$head_branch,worktree_path:$worktree_path,head_sha:$head_sha}'
    exit 0
  fi
  jq -cn \
    --arg current_branch "$head" \
    --arg head_branch "$head" \
    --arg worktree_path "$match_path" \
    --arg head_sha "$match_head" \
    '{ok:true,already_on_pr_branch:true,current_on_pr_branch:false,error:"",reason:"an existing worktree is already on the PR head branch",current_branch:$current_branch,head_branch:$head_branch,worktree_path:$worktree_path,head_sha:$head_sha}'
  exit 0
fi
jq -cn \
  --arg current_branch "$current_branch" \
  --arg head_branch "$head" \
  --arg worktree_path "$repo_root" \
  '{ok:true,already_on_pr_branch:false,current_on_pr_branch:false,error:"",current_branch:$current_branch,head_branch:$head_branch,worktree_path:$worktree_path}'
