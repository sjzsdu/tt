#!/usr/bin/env python3
"""汇总 PR rebase 结果"""
import json, os, traceback

def parse(name, default=None):
    raw = os.environ.get(name, '').strip()
    if not raw:
        return default if default is not None else {}
    try:
        return json.loads(raw)
    except Exception:
        return {'_raw': raw}

try:
    pr = parse('TT_PR', {})
    check = parse('TT_CHECK', {})
    prepare = parse('TT_PREPARE', {})
    attempt = parse('TT_ATTEMPT', {})
    summary = parse('TT_SUMMARY', {})
    validation = parse('TT_VALIDATION', {})
    push = parse('TT_PUSH_RESULT', {})
    cleanup = parse('TT_CLEANUP', {})
    report = os.environ.get('TT_FINAL_REPORT', '').strip()
    already_on_pr_branch = bool(check.get('already_on_pr_branch'))
    rebase_done = bool(summary.get('rebase_done'))
    blocked = bool(summary.get('blocked'))
    status = 'success' if rebase_done else ('already_on_pr_branch' if already_on_pr_branch else ('blocked' if blocked or summary.get('blocker') or attempt.get('error') or prepare.get('error') else 'unknown'))
    result = {
        'pr_number': pr.get('number'),
        'title': pr.get('title') or '',
        'url': pr.get('url') or '',
        'head_branch': pr.get('headRefName') or '',
        'base_branch': prepare.get('base_branch') or pr.get('baseRefName') or pr.get('target_base_branch') or '',
        'status': status,
        'rebase_done': rebase_done,
        'conflict': bool(attempt.get('conflict') or summary.get('conflict_seen')),
        'blocker': summary.get('blocker') or attempt.get('error') or prepare.get('error') or '',
        'already_on_pr_branch': already_on_pr_branch,
        'worktree_path': prepare.get('worktree_path') or check.get('worktree_path') or '',
        'validation_requested': bool(validation.get('requested')),
        'validation_success': validation.get('success'),
        'push_requested': bool(push.get('requested')),
        'pushed': bool(push.get('pushed')),
        'push_remote': push.get('remote') or '',
        'push_branch': push.get('remote_branch') or '',
        'cleanup_removed': cleanup.get('removed'),
        'final_report_excerpt': report[:500],
    }
except Exception as exc:
    result = {'status': 'failed', 'error': f'collect result failed: {exc}', 'traceback': traceback.format_exc()[-2000:]}

print(json.dumps(result, ensure_ascii=False, separators=(',', ':')))
