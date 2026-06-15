#!/usr/bin/env python3
"""提交 inline review comment"""
import json, os, subprocess, sys, traceback

def emit(result):
    print(json.dumps(result, ensure_ascii=False, separators=(',', ':')))

result = {'submitted': False, 'id': '', 'type': 'line', 'path': '', 'line': None, 'side': 'RIGHT', 'error': ''}
try:
    pr_ref = os.environ.get('TT_PR_REF', '').strip()
    repo_hint = os.environ.get('TT_REPO_HINT', '').strip()
    head_sha = os.environ.get('TT_HEAD_SHA', '').strip()
    raw_comment = os.environ.get('TT_REVIEW_COMMENT') or '{}'
    try:
        item = json.loads(raw_comment)
    except Exception as exc:
        result['error'] = f'invalid review_comment JSON: {exc}'
        result['raw_comment_preview'] = raw_comment[:1200]
        emit(result)
        raise SystemExit(0)
    result.update({'id': item.get('id') or '', 'path': item.get('path') or '', 'line': item.get('line'), 'side': item.get('side') or 'RIGHT'})
    body = (item.get('body') or '').strip()
    if not pr_ref:
        result['error'] = 'missing pr_ref'
        emit(result)
        raise SystemExit(0)
    if not head_sha:
        result['error'] = 'missing head_sha'
        emit(result)
        raise SystemExit(0)
    if not body or not result['path'] or result['line'] is None:
        result['error'] = 'missing body/path/line'
        emit(result)
        raise SystemExit(0)
    try:
        line = int(result['line'])
    except Exception:
        result['error'] = 'line must be an integer'
        emit(result)
        raise SystemExit(0)
    if repo_hint and '/' in repo_hint:
        owner, name = repo_hint.split('/', 1)
        endpoint = f'repos/{owner}/{name}/pulls/{pr_ref}/comments'
    else:
        endpoint = f'repos/:owner/:repo/pulls/{pr_ref}/comments'
    cmd = ['gh', 'api', '--method', 'POST', endpoint, '-f', f'body={body}', '-f', f'commit_id={head_sha}', '-f', f'path={result["path"]}', '-F', f'line={line}', '-f', f'side={result["side"]}']
    res = subprocess.run(cmd, capture_output=True, text=True)
    if res.returncode == 0:
        result['submitted'] = True
    else:
        result['error'] = res.stderr.strip() or res.stdout.strip() or 'gh api inline comment failed'
except SystemExit:
    raise
except Exception as exc:
    result['error'] = f'inline comment script failed: {exc}'
    result['traceback'] = traceback.format_exc()[-2000:]
emit(result)
