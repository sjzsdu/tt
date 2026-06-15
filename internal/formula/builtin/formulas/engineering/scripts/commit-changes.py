#!/usr/bin/env python3
"""提交代码"""
import json, subprocess

worktree_path = subprocess.check_output(['git', 'rev-parse', '--show-toplevel'], text=True).strip()
current_branch = subprocess.check_output(['git', 'branch', '--show-current'], text=True).strip()
msg = 'feat: ' + (current_branch.split('/')[-1] if current_branch else 'gongbu')

subprocess.run(['git', 'add', '-A'], check=False)
p = subprocess.run(['git', 'commit', '-m', msg], text=True, capture_output=True)

print(json.dumps({
    'worktree_path': worktree_path,
    'branch_name': current_branch,
    'commit_message': msg,
    'exit_code': p.returncode,
    'stdout': p.stdout,
    'stderr': p.stderr
}, ensure_ascii=False))

raise SystemExit(0)
