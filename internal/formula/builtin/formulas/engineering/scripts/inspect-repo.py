#!/usr/bin/env python3
"""收集仓库状态"""
import json, os, subprocess

def run(args):
    p = subprocess.run(args, text=True, capture_output=True)
    return {'command': args, 'exit_code': p.returncode, 'stdout': p.stdout, 'stderr': p.stderr}

root = subprocess.check_output(['git', 'rev-parse', '--show-toplevel'], text=True).strip()
sparse = run(['git', 'sparse-checkout', 'list'])

print(json.dumps({
    'worktree_path': root,
    'branch_name': subprocess.check_output(['git', 'branch', '--show-current'], text=True).strip(),
    'workspace_cwd': os.environ.get('TT_WORKSPACE_CWD', root),
    'invocation_cwd': os.environ.get('TT_INVOCATION_CWD', ''),
    'formula_run_dir': os.environ.get('TT_FORMULA_RUN_DIR', ''),
    'sparse_paths': [line.strip().strip('/') for line in sparse['stdout'].splitlines() if line.strip()],
    'sparse_enabled': sparse['exit_code'] == 0 and bool(sparse['stdout'].strip()),
    'status': run(['git', 'status', '--short', '--branch']),
    'tracked_files': run(['git', 'ls-files']),
}, ensure_ascii=False))
