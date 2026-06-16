#!/usr/bin/env python3
"""汇总 PR 评论处理结果"""
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
    meta = parse('TT_META', {})
    prepare = parse('TT_PREPARE', {})
    comments = parse('TT_COMMENTS', {})
    normalized = parse('TT_NORMALIZED', {})
    fix_results = parse('TT_FIX_RESULTS', [])
    resolve = parse('TT_RESOLVE', {})
    push = parse('TT_PUSH', {})
    cleanup = parse('TT_CLEANUP', {})
    report = os.environ.get('TT_FINAL_REPORT', '').strip()

    total_comments = int(normalized.get('total') if normalized.get('total') is not None else comments.get('pending_count') or 0)
    worktree_ok = bool(normalized.get('worktree_ok') if 'worktree_ok' in normalized else prepare.get('ok'))
    fixed = not_confirmed = needs_followup = result_count = 0
    result_by_comment = {}
    for row in fix_results if isinstance(fix_results, list) else []:
        obj = row.get('stdout') if isinstance(row, dict) and isinstance(row.get('stdout'), dict) else row
        if not isinstance(obj, dict):
            continue
        status = obj.get('status') or ''
        cid = str(obj.get('comment_id') or '')
        if cid:
            result_by_comment[cid] = obj
        if status:
            result_count += 1
        if status == 'fixed':
            fixed += 1
        elif status == 'not-confirmed':
            not_confirmed += 1
        elif status == 'needs-followup':
            needs_followup += 1

    if meta.get('ok') is False:
        status = 'metadata_failed'
        blocker = meta.get('error') or 'PR metadata unavailable'
    elif prepare.get('ok') is False:
        status = 'worktree_failed'
        blocker = prepare.get('error') or 'worktree preparation failed'
    elif total_comments == 0:
        status = 'no_comments'
        blocker = ''
    elif not worktree_ok:
        status = 'worktree_failed'
        blocker = prepare.get('error') or 'worktree unavailable'
    elif needs_followup:
        status = 'needs_followup'
        blocker = f'{needs_followup} comment(s) need follow-up'
    elif push.get('ok') is False:
        status = 'push_failed'
        blocker = push.get('error') or 'push failed'
    else:
        status = 'completed'
        blocker = ''

    comment_rows = []
    for item in normalized.get('items') or comments.get('comment_items') or []:
        cid = str(item.get('comment_id') or '')
        fix = result_by_comment.get(cid, {})
        comment_rows.append({
            'comment_id': cid,
            'author': item.get('author') or '',
            'path': item.get('path') or '',
            'line': item.get('line') or '',
            'url': item.get('url') or '',
            'status': fix.get('status') or ('not-run' if total_comments else ''),
            'summary': fix.get('summary') or (item.get('body') or '').split('\n', 1)[0][:160],
            'next_action': fix.get('next_action') or '',
        })

    result = {
        'pr_number': pr.get('number') or meta.get('number'),
        'title': pr.get('title') or meta.get('title') or '',
        'url': pr.get('url') or meta.get('url') or '',
        'head_branch': pr.get('headRefName') or meta.get('headRefName') or '',
        'base_branch': pr.get('baseRefName') or meta.get('baseRefName') or '',
        'status': status,
        'blocker': blocker,
        'pending_comments': total_comments,
        'processed_comments': result_count,
        'fixed': fixed,
        'not_confirmed': not_confirmed,
        'needs_followup': needs_followup,
        'worktree_ok': worktree_ok,
        'workspace_path': normalized.get('workspace_path') or prepare.get('workspace_path') or '',
        'reused_current': bool(prepare.get('reused_current')),
        'reused_worktree': bool(prepare.get('reused_worktree')),
        'created': bool(prepare.get('created')),
        'resolved_threads': resolve.get('resolved_count'),
        'resolve_failed': resolve.get('failed_count'),
        'push_ok': push.get('ok'),
        'pushed': bool(push.get('pushed')),
        'committed': bool(push.get('committed')),
        'changed': bool(push.get('changed')),
        'remote_synced': push.get('remote_synced'),
        'local_head': push.get('local_head') or '',
        'remote_head': push.get('remote_head') or '',
        'commit_sha': push.get('commit_sha') or '',
        'cleanup_removed': cleanup.get('removed'),
        'cleanup_attempted': cleanup.get('attempted'),
        'cleanup_error': cleanup.get('stderr') or '',
        'comments': comment_rows,
        'final_report_excerpt': report[:500],
    }
except Exception as exc:
    result = {'status': 'failed', 'error': f'collect result failed: {exc}', 'traceback': traceback.format_exc()[-2000:]}

print(json.dumps(result, ensure_ascii=False, separators=(',', ':')))
