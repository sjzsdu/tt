#!/bin/bash
# 准备 PR 修复 worktree
set -u

meta="${TT_PR_META:-{}}"
repo_hint="${TT_REPO_HINT:-}"
root_input="${TT_WORKTREE_ROOT:-.tt/worktrees/github-pr-fix-comments}"

ok="$(jq -r '.ok // false' <<<"$meta")"
if [ "$ok" != true ]; then
  jq -cn --arg error "pr metadata unavailable" --arg metadata_raw "$meta" '{ok:false,error:$error,metadata_raw:$metadata_raw,workspace_path:"",reused_current:false,reused_worktree:false,created:false}'
  exit 0
fi

number="$(jq -r '.number // empty' <<<"$meta")"
head="$(jq -r '.headRefName // empty' <<<"$meta")"
base="$(jq -r '.baseRefName // empty' <<<"$meta")"

if [ -z "$number" ] || [ -z "$head" ]; then
  jq -cn --arg error "missing PR number/head branch" --arg metadata_raw "$meta" '{ok:false,error:$error,metadata_raw:$metadata_raw,workspace_path:"",reused_current:false,reused_worktree:false,created:false}'
  exit 0
fi

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
invocation_dir="$(pwd)"
current_branch="$(git -C "$repo_root" branch --show-current 2>/dev/null || true)"
safe_head="$(printf '%s' "$head" | tr -c 'A-Za-z0-9._-' '-')"

worktree_root="$root_input"
case "$worktree_root" in /*) ;; *) worktree_root="$invocation_dir/$worktree_root" ;; esac

target="$worktree_root/pr-${number}-${safe_head}"
pr_head_ref="refs/tt/pr-fix-comments/${number}/head"
remote_head_ref="refs/remotes/origin/${head}"

mkdir -p "$worktree_root"

fetch_pr_head() {
  source_ref="$remote_head_ref"
  source_kind="origin_branch"
  if fetch_out="$(git -C "$repo_root" fetch origin "+refs/heads/${head}:${remote_head_ref}" --prune 2>&1)"; then
    return 0
  fi
  origin_fetch_error="$fetch_out"
  source_ref="$pr_head_ref"
  source_kind="pull_ref"
  if fetch_out="$(git -C "$repo_root" fetch origin "+refs/pull/${number}/head:${pr_head_ref}" --prune 2>&1)"; then
    return 0
  fi
  fetch_out="origin branch fetch failed: ${origin_fetch_error}; PR head fetch failed: ${fetch_out}"
  return 1
}

if [ -d "$target/.git" ] || [ -f "$target/.git" ]; then
  if ! git -C "$target" diff --quiet || ! git -C "$target" diff --cached --quiet; then
    jq -cn --arg error "existing target worktree has uncommitted changes before PR checkout" --arg path "$target" '{ok:false,error:$error,workspace_path:$path,reused_current:false,reused_worktree:true,created:false}'
    exit 0
  fi
  if ! fetch_pr_head; then
    jq -cn --arg error "git fetch PR head failed: $fetch_out" --arg path "$target" '{ok:false,error:$error,workspace_path:$path,reused_current:false,reused_worktree:true,created:false}'
    exit 0
  fi
  if ! checkout_out="$(git -C "$target" checkout --detach "$source_ref" 2>&1)"; then
    jq -cn --arg error "git checkout PR head failed: $checkout_out" --arg path "$target" '{ok:false,error:$error,workspace_path:$path,reused_current:false,reused_worktree:true,created:false}'
    exit 0
  fi
  source_sha="$(git -C "$repo_root" rev-parse "$source_ref" 2>/dev/null || true)"
  jq -cn --arg path "$target" --arg repo_root "$repo_root" --arg branch "$head" --arg base "$base" --arg source_ref "$source_ref" --arg source_kind "$source_kind" --arg source_sha "$source_sha" '{ok:true,error:"",workspace_path:$path,repo_root:$repo_root,head_branch:$branch,base_branch:$base,source_ref:$source_ref,source_kind:$source_kind,source_sha:$source_sha,reused_current:false,reused_worktree:true,created:false,cleanup_required:true,detached:true}'
  exit 0
fi

if ! fetch_pr_head; then
  jq -cn --arg error "git fetch PR head failed: $fetch_out" '{ok:false,error:$error,workspace_path:"",reused_current:false,reused_worktree:false,created:false}'
  exit 0
fi

if ! git -C "$repo_root" worktree add --detach "$target" "$source_ref" >/tmp/tt-pr-fix-worktree.log 2>&1; then
  err="$(cat /tmp/tt-pr-fix-worktree.log 2>/dev/null || true)"
  jq -cn --arg error "git worktree add failed: $err" --arg path "$target" '{ok:false,error:$error,workspace_path:$path,reused_current:false,reused_worktree:false,created:false}'
  exit 0
fi

source_sha="$(git -C "$repo_root" rev-parse "$source_ref" 2>/dev/null || true)"
jq -cn --arg path "$target" --arg repo_root "$repo_root" --arg branch "$head" --arg base "$base" --arg source_ref "$source_ref" --arg source_kind "$source_kind" --arg source_sha "$source_sha" '{ok:true,error:"",workspace_path:$path,repo_root:$repo_root,head_branch:$branch,base_branch:$base,source_ref:$source_ref,source_kind:$source_kind,source_sha:$source_sha,reused_current:false,reused_worktree:false,created:true,cleanup_required:true,detached:true}'
