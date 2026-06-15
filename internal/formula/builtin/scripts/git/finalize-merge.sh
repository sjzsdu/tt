#!/bin/bash
# 完成合并
set -u

repo_root="${TT_REPO_ROOT:-}"
branch="${TT_BRANCH:-}"
index="${TT_INDEX:-}"
attempt_raw="${TT_ATTEMPT:-}"
resolve_raw="${TT_RESOLVE:-}"

cd "$repo_root" 2>/dev/null || {
  echo "{\"branch\":\"$branch\",\"merged\":false,\"skipped\":true,\"error\":\"repo root missing\"}"
  exit 0
}

attempt="$(jq -cn --arg s "$attempt_raw" 'if ($s|length)==0 then {} else (($s|fromjson?)//{}) end | if type=="object" and (.stdout|type)=="object" then .stdout elif type=="object" and (.stdout|type)=="string" then ((.stdout|fromjson?)//.) else . end')"
needs_resolution="$(jq -r '.needs_resolution // false' <<<"$attempt")"

if [ "$needs_resolution" != true ]; then
  merged="$(jq -r '.merged // false' <<<"$attempt")"
  skipped="$(jq -r '.skipped // false' <<<"$attempt")"
  jq -cn \
    --arg branch "$branch" \
    --arg index "$index" \
    --argjson attempt "$attempt" \
    --arg head "$(git rev-parse HEAD 2>/dev/null || true)" \
    '{branch:$branch,index:($index|tonumber? // 0),final:true,merged:($attempt.merged // false),conflict:false,skipped:($attempt.skipped // false),skip_reason:(if ($attempt.skipped // false) then ($attempt.error // "merge_failed") else "" end),conflict_files:[],resolved_files:[],head:$head,attempt:$attempt,resolve:{},continue_output:""}'
  exit 0
fi

initial_conflicts="$(jq -c '.conflict_files // []' <<<"$attempt")"
marker_files="$(jq -r '.[]' <<<"$initial_conflicts" | while IFS= read -r f; do [ -f "$f" ] && git grep -n -E '^(<<<<<<<|=======|>>>>>>>)' -- "$f" 2>/dev/null | cut -d: -f1 || true; done | sort -u | jq -R -s -c 'split("\n") | map(select(length>0))')"
unmerged_files="$(git diff --name-only --diff-filter=U | jq -R -s -c 'split("\n") | map(select(length>0))')"

if [ "$(jq 'length' <<<"$marker_files")" != "0" ]; then
  abort_out="$(git merge --abort 2>&1)"
  jq -cn \
    --arg branch "$branch" \
    --arg index "$index" \
    --arg abort_output "$abort_out" \
    --argjson attempt "$attempt" \
    --arg resolve_raw "$resolve_raw" \
    --argjson conflict_files "$initial_conflicts" \
    --argjson remaining_marker_files "$marker_files" \
    '{branch:$branch,index:($index|tonumber? // 0),final:true,merged:false,conflict:true,skipped:true,skip_reason:"conflict_markers_remain",conflict_files:$conflict_files,remaining_marker_files:$remaining_marker_files,resolved_files:[],head:"",attempt:$attempt,resolve_raw:$resolve_raw,abort_output:$abort_output,continue_output:""}'
  exit 0
fi

if [ "$(jq 'length' <<<"$unmerged_files")" != "0" ]; then
  jq -r '.[]' <<<"$unmerged_files" | while IFS= read -r f; do [ -n "$f" ] && git add -- "$f"; done
fi

if [ -f "$(git rev-parse --git-path MERGE_HEAD 2>/dev/null)" ]; then
  cont_out="$(GIT_EDITOR=true git merge --continue 2>&1)"
  cont_rc=$?
else
  cont_out="merge already completed"
  cont_rc=0
fi

if [ "$cont_rc" -eq 0 ]; then
  jq -cn \
    --arg branch "$branch" \
    --arg index "$index" \
    --arg head "$(git rev-parse HEAD 2>/dev/null || true)" \
    --arg continue_output "$cont_out" \
    --argjson attempt "$attempt" \
    --arg resolve_raw "$resolve_raw" \
    --argjson conflict_files "$initial_conflicts" \
    --argjson resolved_files "$unmerged_files" \
    '{branch:$branch,index:($index|tonumber? // 0),final:true,merged:true,conflict:true,skipped:false,skip_reason:"",conflict_files:$conflict_files,resolved_files:$resolved_files,head:$head,attempt:$attempt,resolve_raw:$resolve_raw,continue_output:$continue_output}'
  exit 0
fi

abort_out="$(git merge --abort 2>&1)"
jq -cn \
  --arg branch "$branch" \
  --arg index "$index" \
  --arg continue_output "$cont_out" \
  --arg abort_output "$abort_out" \
  --argjson attempt "$attempt" \
  --arg resolve_raw "$resolve_raw" \
  --argjson conflict_files "$initial_conflicts" \
  '{branch:$branch,index:($index|tonumber? // 0),final:true,merged:false,conflict:true,skipped:true,skip_reason:"merge_continue_failed",conflict_files:$conflict_files,resolved_files:[],head:"",attempt:$attempt,resolve_raw:$resolve_raw,continue_output:$continue_output,abort_output:$abort_output}'
