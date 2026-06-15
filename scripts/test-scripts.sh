#!/bin/bash
# Formula 脚本输出验证测试
# 用于验证各个脚本的输出格式是否符合预期

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FORMULA_DIR="$(cd "$SCRIPT_DIR/../internal/formula/builtin" && pwd)"
SCRIPTS_DIR="$FORMULA_DIR/scripts"
FORMULAS_DIR="$FORMULA_DIR/formulas"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试计数器
TOTAL=0
PASSED=0
FAILED=0

# 测试函数
test_output() {
  local name="$1"
  local expected="$2"
  local actual="$3"
  
  TOTAL=$((TOTAL + 1))
  # 将 Python 的 True/False 转换为 true/false
  local normalized_actual=$(echo "$actual" | sed 's/^True$/true/;s/^False$/false/')
  local normalized_expected=$(echo "$expected" | sed 's/^True$/true/;s/^False$/false/')
  
  if [ "$normalized_expected" = "$normalized_actual" ]; then
    echo -e "${GREEN}✓${NC} $name"
    PASSED=$((PASSED + 1))
  else
    echo -e "${RED}✗${NC} $name"
    echo "  Expected: $normalized_expected"
    echo "  Actual:   $normalized_actual"
    FAILED=$((FAILED + 1))
  fi
}

# 验证 JSON 格式
validate_json() {
  local name="$1"
  local json="$2"
  local required_fields="${3:-}"
  
  TOTAL=$((TOTAL + 1))
  
  # 检查是否是有效 JSON
  if ! echo "$json" | python3 -m json.tool >/dev/null 2>&1; then
    echo -e "${RED}✗${NC} $name: Invalid JSON"
    echo "  Input: ${json:0:200}..."
    FAILED=$((FAILED + 1))
    return 1
  fi
  
  # 检查必需字段
  if [ -n "$required_fields" ]; then
    for field in $required_fields; do
      if ! echo "$json" | python3 -c "import json,sys; d=json.load(sys.stdin); sys.exit(0 if '$field' in d else 1)" 2>/dev/null; then
        echo -e "${RED}✗${NC} $name: Missing required field '$field'"
        FAILED=$((FAILED + 1))
        return 1
      fi
    done
  fi
  
  echo -e "${GREEN}✓${NC} $name"
  PASSED=$((PASSED + 1))
  return 0
}

# 验证 JSON 数组
validate_json_array() {
  local name="$1"
  local json="$2"
  local min_items="${3:-0}"
  local max_items="${4:-999999}"
  
  TOTAL=$((TOTAL + 1))
  
  count=$(echo "$json" | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d) if isinstance(d, list) else 0)" 2>/dev/null || echo "0")
  
  if [ "$count" -lt "$min_items" ] || [ "$count" -gt "$max_items" ]; then
    echo -e "${RED}✗${NC} $name: Array length $count not in range [$min_items, $max_items]"
    FAILED=$((FAILED + 1))
    return 1
  fi
  
  echo -e "${GREEN}✓${NC} $name (items: $count)"
  PASSED=$((PASSED + 1))
  return 0
}

# 测试共享脚本
echo -e "\n${YELLOW}=== 共享脚本测试 ===${NC}"

# 测试 check-status.sh
echo -e "\n${YELLOW}Testing check-status.sh${NC}"
result=$(bash "$SCRIPTS_DIR/git/check-status.sh" 2>&1 || true)
validate_json "check-status output" "$result" "ok repo_root current_branch"

# 测试 run-validation.sh
echo -e "\n${YELLOW}Testing run-validation.sh${NC}"
result=$(TT_VALIDATION_COMMAND="-" bash "$SCRIPTS_DIR/validation/run-validation.sh" 2>&1 || true)
validate_json "run-validation (skip)" "$result" "ok"
test_output "run-validation skip value" "true" "$(echo "$result" | python3 -c "import json,sys; print(json.load(sys.stdin).get('skipped', ''))" 2>/dev/null)"

# 测试 git 脚本
echo -e "\n${YELLOW}=== Git 脚本测试 ===${NC}"

# 测试 prepare-merge-branch.sh (模拟环境)
echo -e "\n${YELLOW}Testing prepare-merge-branch.sh${NC}"
# 创建测试分支
test_branch="test-merge-$(date +%s)"
git branch "$test_branch" 2>/dev/null || true

result=$(TT_BRANCHES="$test_branch" TT_TARGET_BRANCH="tmp-test-merge" bash "$SCRIPTS_DIR/git/prepare-merge-branch.sh" 2>&1 || true)
validate_json "prepare-merge-branch output" "$result" "ok"
# 注意：如果工作区不干净，items 可能为空
validate_json_array "prepare-merge-branch items" "$(echo "$result" | python3 -c "import json,sys; print(json.dumps(json.load(sys.stdin).get('items', [])))" 2>/dev/null)" 0

# 清理
git branch -D "$test_branch" 2>/dev/null || true
git branch -D "tmp-test-merge" 2>/dev/null || true

# 测试 github 脚本
echo -e "\n${YELLOW}=== GitHub 脚本测试 ===${NC}"

# 测试 fetch-review-comments.py (模拟环境)
echo -e "\n${YELLOW}Testing fetch-review-comments.py${NC}"
result=$(TT_PR_META='{"ok":false}' TT_REPO_HINT="" python3 "$FORMULAS_DIR/github/scripts/fetch-review-comments.py" 2>&1 || true)
validate_json "fetch-review-comments (no PR)" "$result" "ok comment_items"
test_output "fetch-review-comments error handling" "false" "$(echo "$result" | python3 -c "import json,sys; print(json.load(sys.stdin).get('ok', ''))" 2>/dev/null)"

# 测试 normalize-comment-items.py
echo -e "\n${YELLOW}Testing normalize-comment-items.py${NC}"
result=$(TT_COMMENTS='{"comment_items":[]}' TT_WORKTREE='{"ok":false}' TT_REVIEW_FOCUS="test" python3 "$FORMULAS_DIR/github/scripts/normalize-comment-items.py" 2>&1 || true)
validate_json "normalize-comment-items (empty)" "$result" "items should_process"
test_output "normalize-comment-items should_process" "false" "$(echo "$result" | python3 -c "import json,sys; print(json.load(sys.stdin).get('should_process', ''))" 2>/dev/null)"

# 测试 docs 脚本
echo -e "\n${YELLOW}=== Docs 脚本测试 ===${NC}"

# 测试 repo-map.py (模拟环境)
echo -e "\n${YELLOW}Testing repo-map.py${NC}"
result=$(TT_REPO_PATH="$(pwd)" TT_ANALYSIS_PATH="$(pwd)" python3 "$FORMULAS_DIR/docs/scripts/repo-map.py" 2>&1 || true)
validate_json "repo-map output" "$result" "ok repo_path analysis_path complexity stats"

# 测试 engineering 脚本
echo -e "\n${YELLOW}=== Engineering 脚本测试 ===${NC}"

# 测试 inspect-repo.py
echo -e "\n${YELLOW}Testing inspect-repo.py${NC}"
result=$(python3 "$FORMULAS_DIR/engineering/scripts/inspect-repo.py" 2>&1 || true)
validate_json "inspect-repo output" "$result" "worktree_path branch_name"

# 测试 prepare-conflict-context.py (模拟无冲突环境)
echo -e "\n${YELLOW}Testing prepare-conflict-context.py${NC}"
result=$(python3 "$FORMULAS_DIR/engineering/scripts/prepare-conflict-context.py" 2>&1 || true)
validate_json "prepare-conflict-context output" "$result" "ok needs_resolution done"
test_output "prepare-conflict-context no conflict" "true" "$(echo "$result" | python3 -c "import json,sys; print(json.load(sys.stdin).get('done', ''))" 2>/dev/null)"

# 测试 verify-conflict-resolution.sh (模拟无冲突环境)
echo -e "\n${YELLOW}Testing verify-conflict-resolution.sh${NC}"
result=$(TT_REPO_ROOT="$(pwd)" TT_PREPARE='{"done":true,"files":[],"operation":"none","started_at":"","started_epoch":0}' TT_RESOLVE='[]' bash "$FORMULAS_DIR/engineering/scripts/verify-conflict-resolution.sh" 2>&1 || true)
validate_json "verify-conflict-resolution output" "$result" "resolved operation"
test_output "verify-conflict-resolution resolved" "true" "$(echo "$result" | python3 -c "import json,sys; print(json.load(sys.stdin).get('resolved', ''))" 2>/dev/null)"

# 测试 commit-changes.py (模拟环境)
echo -e "\n${YELLOW}Testing commit-changes.py${NC}"
result=$(python3 "$FORMULAS_DIR/engineering/scripts/commit-changes.py" 2>&1 || true)
validate_json "commit-changes output" "$result" "worktree_path branch_name commit_message exit_code"

# 汇总结果
echo -e "\n${YELLOW}=== 测试汇总 ===${NC}"
echo "Total:  $TOTAL"
echo -e "${GREEN}Passed: $PASSED${NC}"
if [ $FAILED -gt 0 ]; then
  echo -e "${RED}Failed: $FAILED${NC}"
  exit 1
else
  echo -e "${GREEN}All tests passed!${NC}"
  exit 0
fi
