#!/bin/bash
# 通用 git 操作检查脚本
# 检查当前 git 仓库状态
set -euo pipefail

action="${TT_ACTION:-status}"

json_error() {
  local msg="$1"
  echo "{\"ok\":false,\"error\":\"$msg\"}"
  exit 1
}

# 检查是否在 git 仓库内
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
[ -n "$repo_root" ] || json_error "not inside a git repository"
cd "$repo_root"

case "$action" in
  status)
    current_branch="$(git branch --show-current 2>/dev/null || true)"
    has_uncommitted=false
    ! git diff --quiet 2>/dev/null && has_uncommitted=true
    has_staged=false
    ! git diff --cached --quiet 2>/dev/null && has_staged=true
    in_merge=false
    [ -f "$(git rev-parse --git-path MERGE_HEAD 2>/dev/null)" ] && in_merge=true
    in_rebase=false
    [ -d "$(git rev-parse --git-path rebase-merge 2>/dev/null)" ] && in_rebase=true
    [ -d "$(git rev-parse --git-path rebase-apply 2>/dev/null)" ] && in_rebase=true
    in_cherry_pick=false
    [ -f "$(git rev-parse --git-path CHERRY_PICK_HEAD 2>/dev/null)" ] && in_cherry_pick=true
    
    jq -n \
      --arg repo_root "$repo_root" \
      --arg current_branch "$current_branch" \
      --argjson has_uncommitted "$has_uncommitted" \
      --argjson has_staged "$has_staged" \
      --argjson in_merge "$in_merge" \
      --argjson in_rebase "$in_rebase" \
      --argjson in_cherry_pick "$in_cherry_pick" \
      '{ok:true,repo_root:$repo_root,current_branch:$current_branch,has_uncommitted:$has_uncommitted,has_staged:$has_staged,in_merge:$in_merge,in_rebase:$in_rebase,in_cherry_pick:$in_cherry_pick}'
    ;;
    
  check-clean)
    if ! git diff --quiet 2>/dev/null || ! git diff --cached --quiet 2>/dev/null; then
      echo '{"ok":false,"error":"working tree or index is not clean"}'
      exit 1
    fi
    echo '{"ok":true,"clean":true}'
    ;;
    
  check-operation)
    if [ -f "$(git rev-parse --git-path MERGE_HEAD 2>/dev/null)" ] || \
       [ -d "$(git rev-parse --git-path rebase-merge 2>/dev/null)" ] || \
       [ -d "$(git rev-parse --git-path rebase-apply 2>/dev/null)" ] || \
       [ -f "$(git rev-parse --git-path CHERRY_PICK_HEAD 2>/dev/null)" ]; then
      echo '{"ok":false,"error":"git operation already in progress"}'
      exit 1
    fi
    echo '{"ok":true,"no_operation_in_progress":true}'
    ;;
    
  *)
    json_error "unknown action: $action"
    ;;
esac
