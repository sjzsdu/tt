#!/bin/bash
# 通用验证脚本
# 运行指定的验证命令
set -u

command="${TT_VALIDATION_COMMAND:-}"
repo_path="${TT_REPO_PATH:-}"
timeout="${TT_TIMEOUT:-300}"

json_error() {
  local msg="$1"
  echo "{\"ok\":false,\"error\":\"$msg\",\"success\":false}"
  exit 1
}

if [ -z "$command" ] || [ "$command" = "-" ]; then
  echo '{"ok":true,"skipped":true,"reason":"no validation command specified"}'
  exit 0
fi

if [ -n "$repo_path" ] && [ -d "$repo_path" ]; then
  cd "$repo_path"
fi

# 运行验证命令
start_time=$(date +%s)
eval "timeout $timeout $command" 2>&1
rc=$?
end_time=$(date +%s)
duration=$((end_time - start_time))

if [ "$rc" -eq 0 ]; then
  echo "{\"ok\":true,\"success\":true,\"command\":\"$command\",\"duration\":$duration}"
else
  echo "{\"ok\":true,\"success\":false,\"command\":\"$command\",\"exit_code\":$rc,\"duration\":$duration}"
fi
