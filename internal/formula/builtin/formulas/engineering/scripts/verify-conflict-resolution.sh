#!/bin/bash
# 确认冲突已解决
set -euo pipefail

repo_root="${TT_REPO_ROOT:-}"
[ -n "$repo_root" ] && cd "$repo_root"

prepare_raw="${TT_PREPARE:-}"
resolve_raw="${TT_RESOLVE:-}"

prepare_json="$(jq -cn --arg s "$prepare_raw" 'if ($s|length)==0 then {} else (($s|fromjson?)//{}) end | if type=="object" and (.stdout|type=="object") then .stdout elif type=="object" and (.stdout|type=="string") then ((.stdout|fromjson?)//.) else . end')"
resolve_json="$(jq -cn --arg s "$resolve_raw" 'if ($s|length)==0 then [] else (($s|fromjson?)//[]) end | if type=="object" and (.stdout|type)=="array" then .stdout elif type=="object" and (.stdout|type)=="string" then ((.stdout|fromjson?)//[]) elif type=="array" then . else [] end')"

initial_conflicts="$(jq -c '.files // .conflicted_files // []' <<<"$prepare_json")"
operation="$(jq -r '.operation // "none"' <<<"$prepare_json")"
started_at="$(jq -r '.started_at // ""' <<<"$prepare_json")"
started_epoch="$(jq -r '.started_epoch // 0' <<<"$prepare_json")"
completed_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
completed_epoch="$(date +%s)"
elapsed_seconds=$(( completed_epoch - started_epoch )); [ "$elapsed_seconds" -lt 0 ] && elapsed_seconds=0

unmerged_index_files="$(git diff --name-only --diff-filter=U | jq -R -s -c 'split("\n") | map(select(length>0))')"
marker_files="$({ jq -r '.[]' <<<"$initial_conflicts" | while IFS= read -r file; do [ -f "$file" ] && git grep -l -E '^(<<<<<<<|=======|>>>>>>>)' -- "$file" 2>/dev/null || true; done; } | sort -u | jq -R -s -c 'split("\n") | map(select(length>0))')"
resolver_unresolved="$(jq -c '[.[] | select((.resolved != true) or ((.blocker_type // "none") != "none")) | (.conflicted_files // .touched_files // [])[]] | unique' <<<"$resolve_json")"

remaining="$(jq -cn --argjson a "$marker_files" '$a|unique')"

if [ "$(jq -r 'length' <<<"$remaining")" -gt 0 ]; then
  jq -cn \
    --arg operation "$operation" \
    --arg started_at "$started_at" \
    --arg completed_at "$completed_at" \
    --argjson elapsed_seconds "$elapsed_seconds" \
    --argjson initial_conflicts "$initial_conflicts" \
    --argjson remaining "$remaining" \
    --argjson marker_files "$marker_files" \
    --argjson resolver_unresolved "$resolver_unresolved" \
    --argjson unmerged_index_files "$unmerged_index_files" \
    '{resolved:false,operation:$operation,started_at:$started_at,completed_at:$completed_at,elapsed_seconds:$elapsed_seconds,initial_conflicts:$initial_conflicts,remaining_conflicts:$remaining,remaining_marker_files:$marker_files,resolver_unresolved_files:$resolver_unresolved,unmerged_index_files:$unmerged_index_files,continued:false,blocker_type:"unresolved_conflict_markers",blocker_summary:"conflict markers remain in working tree"}'
  exit 1
fi

resolved_files="$(jq -c <<<"$initial_conflicts")"
jq -cn \
  --arg operation "$operation" \
  --arg head "$(git rev-parse HEAD)" \
  --arg started_at "$started_at" \
  --arg completed_at "$completed_at" \
  --argjson elapsed_seconds "$elapsed_seconds" \
  --argjson initial_conflicts "$initial_conflicts" \
  --argjson resolved_files "$resolved_files" \
  --argjson resolver_unresolved "$resolver_unresolved" \
  --argjson unmerged_index_files "$unmerged_index_files" \
  '{resolved:true,operation:$operation,started_at:$started_at,completed_at:$completed_at,elapsed_seconds:$elapsed_seconds,initial_conflicts:$initial_conflicts,remaining_conflicts:[],remaining_marker_files:[],resolver_unresolved_files:$resolver_unresolved,resolved_files:$resolved_files,needs_user_add:$resolved_files,unmerged_index_files:$unmerged_index_files,continued:false,head:$head,blocker_type:"none",blocker_summary:""}'
