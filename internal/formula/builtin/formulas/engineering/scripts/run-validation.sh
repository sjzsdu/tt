#!/bin/bash
# 运行可选验证命令
set -u

repo_root="${TT_REPO_ROOT:-}"
cmd="${TT_VALIDATION_COMMAND:-}"

[ -n "$repo_root" ] && cd "$repo_root"

if [ -z "$cmd" ] || [ "$cmd" = "-" ]; then
  jq -cn '{attempted:false,success:true,command:"",stdout:"",stderr:"",exit_code:0}'
  exit 0
fi

out="$(bash -lc "$cmd" 2>&1)"
rc=$?

jq -cn \
  --arg command "$cmd" \
  --arg output "$out" \
  --argjson exit_code "$rc" \
  '{attempted:true,success:($exit_code==0),command:$command,stdout:(if $exit_code==0 then $output else "" end),stderr:(if $exit_code==0 then "" else $output end),exit_code:$exit_code}'

exit "$rc"
