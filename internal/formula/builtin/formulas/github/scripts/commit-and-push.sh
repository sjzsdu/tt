#!/bin/bash
# 提交必要修改并 push PR 分支
set -u

workspace="${TT_WORKSPACE_PATH:-}"
head="${TT_HEAD_BRANCH:-}"

if [ -z "$workspace" ] || [ ! -d "$workspace" ]; then
  jq -cn --arg error "workspace_path unavailable" '{ok:false,error:$error,pushed:false,committed:false,changed:false}'
  exit 0
fi

if [ -z "$head" ]; then
  jq -cn --arg error "head branch unavailable" '{ok:false,error:$error,pushed:false,committed:false,changed:false}'
  exit 0
fi

status_before="$(git -C "$workspace" status --porcelain)"
tracked_status="$(git -C "$workspace" status --porcelain --untracked-files=no)"
untracked_status="$(git -C "$workspace" ls-files --others --exclude-standard)"

committed=false
commit_sha=""
commit_out=""

if [ -n "$tracked_status" ]; then
  git -C "$workspace" add -u
  commit_out="$(git -C "$workspace" commit --no-verify -m "fix: address PR review comments" 2>&1)"
  rc=$?
  if [ "$rc" -ne 0 ]; then
    jq -cn --arg error "git commit failed: $commit_out" --arg status "$status_before" --arg tracked_status "$tracked_status" --arg untracked_status "$untracked_status" '{ok:false,error:$error,pushed:false,committed:false,changed:true,tracked_changed:($tracked_status != ""),untracked_ignored:$untracked_status,status_before:$status,tracked_status:$tracked_status}'
    exit 0
  fi
  committed=true
  commit_sha="$(git -C "$workspace" rev-parse HEAD 2>/dev/null || true)"
fi

push_out="$(git -C "$workspace" push --no-verify origin "HEAD:${head}" 2>&1)"
push_rc=$?

if [ "$push_rc" -ne 0 ]; then
  jq -cn --arg error "git push failed: $push_out" --arg commit_sha "$commit_sha" --argjson committed "$committed" --arg status "$status_before" --arg tracked_status "$tracked_status" --arg untracked_status "$untracked_status" '{ok:false,error:$error,pushed:false,committed:$committed,commit_sha:$commit_sha,changed:($status != ""),tracked_changed:($tracked_status != ""),untracked_ignored:$untracked_status,status_before:$status,tracked_status:$tracked_status}'
  exit 0
fi

local_head="$(git -C "$workspace" rev-parse HEAD 2>/dev/null || true)"
verify_out="$(git -C "$workspace" fetch origin "+refs/heads/${head}:refs/remotes/origin/${head}" 2>&1)"
verify_rc=$?
remote_head="$(git -C "$workspace" rev-parse "refs/remotes/origin/${head}" 2>/dev/null || true)"

remote_synced=false
if [ "$verify_rc" -eq 0 ] && [ -n "$local_head" ] && [ "$local_head" = "$remote_head" ]; then
  remote_synced=true
fi

if [ "$remote_synced" != true ]; then
  jq -cn --arg error "remote branch sync verification failed" --arg push_stdout "$push_out" --arg verify_stdout "$verify_out" --arg local_head "$local_head" --arg remote_head "$remote_head" --arg commit_sha "$commit_sha" --argjson committed "$committed" --arg status "$status_before" --arg tracked_status "$tracked_status" --arg untracked_status "$untracked_status" '{ok:false,error:$error,pushed:true,remote_synced:false,local_head:$local_head,remote_head:$remote_head,committed:$committed,commit_sha:$commit_sha,changed:($status != ""),tracked_changed:($tracked_status != ""),untracked_ignored:$untracked_status,status_before:$status,tracked_status:$tracked_status,push_stdout:$push_out,verify_stdout:$verify_out}'
  exit 0
fi

jq -cn --arg push_stdout "$push_out" --arg verify_stdout "$verify_out" --arg local_head "$local_head" --arg remote_head "$remote_head" --arg commit_stdout "$commit_out" --arg commit_sha "$commit_sha" --argjson committed "$committed" --arg status "$status_before" --arg tracked_status "$tracked_status" --arg untracked_status "$untracked_status" '{ok:true,error:"",pushed:true,remote_synced:true,local_head:$local_head,remote_head:$remote_head,committed:$committed,commit_sha:$commit_sha,changed:($status != ""),tracked_changed:($tracked_status != ""),untracked_ignored:$untracked_status,status_before:$status,tracked_status:$tracked_status,commit_stdout:$commit_stdout,push_stdout:$push_stdout,verify_stdout:$verify_stdout}'
