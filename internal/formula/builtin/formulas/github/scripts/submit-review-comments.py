#!/usr/bin/env python3
"""提交 review summary 和 conversation comments"""
import json, subprocess, sys, traceback

def emit(result):
    print(json.dumps(result, ensure_ascii=False, separators=(',', ':')))

result = {'submitted': False, 'review_action': 'COMMENT', 'summary_comment': '', 'submitted_comments': [], 'failed_comments': [], 'conversation_comments_requested': 0, 'review_error': ''}
try:
    pr_ref = sys.argv[1] if len(sys.argv) > 1 else ''
    repo_hint = (sys.argv[2] if len(sys.argv) > 2 else '').strip()
    raw_plan = sys.argv[3] if len(sys.argv) > 3 else '{}'
    try:
        review_plan = json.loads(raw_plan or '{}')
    except Exception as exc:
        result['review_error'] = f'invalid review plan JSON: {exc}'
        result['raw_plan_preview'] = raw_plan[:1200]
        emit(result)
        raise SystemExit(0)
    result['summary_comment'] = review_plan.get('summary_comment', '')
    conversation_comments = review_plan.get('conversation_comments') or []
    result['conversation_comments_requested'] = len(conversation_comments)
    repo_args = ['--repo', repo_hint] if repo_hint else []
    if not pr_ref:
        result['review_error'] = 'missing pr_ref'
        emit(result)
        raise SystemExit(0)
    if not review_plan.get('has_findings', False):
        result['reason'] = 'No findings to submit'
        emit(result)
        raise SystemExit(0)

    body = (review_plan.get('summary_comment') or '').strip() or 'Automated PR review summary.'
    res = subprocess.run(['gh', 'pr', 'review', pr_ref, '--comment', '--body', body] + repo_args, capture_output=True, text=True)
    if res.returncode == 0:
        result['submitted'] = True
        result['submitted_review_body'] = True
    else:
        result['review_error'] = res.stderr.strip() or res.stdout.strip() or 'gh pr review failed'

    for item in conversation_comments:
        cbody = (item.get('body') or '').strip()
        if not cbody:
            continue
        cres = subprocess.run(['gh', 'pr', 'comment', pr_ref, '--body', cbody] + repo_args, capture_output=True, text=True)
        if cres.returncode == 0:
            result['submitted_comments'].append({'id': item.get('id'), 'type': 'conversation'})
        else:
            result['failed_comments'].append({'id': item.get('id'), 'type': 'conversation', 'error': cres.stderr.strip() or cres.stdout.strip() or 'gh pr comment failed'})
except SystemExit:
    raise
except Exception as exc:
    result['review_error'] = f'submit summary script failed: {exc}'
    result['traceback'] = traceback.format_exc()[-2000:]
emit(result)
