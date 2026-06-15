#!/usr/bin/env python3
"""resolve 已处理的 GitHub review threads"""
import json, os, subprocess

def emit(obj):
    print(json.dumps(obj, ensure_ascii=False, separators=(',', ':')))

def load(name, default):
    raw = os.environ.get(name, '').strip()
    if not raw:
        return default
    try:
        return json.loads(raw)
    except Exception:
        return default

items = load('TT_ITEMS', {}).get('items') or []
results = load('TT_RESULTS', [])
status_by_comment = {}

for row in results if isinstance(results, list) else []:
    obj = row.get('stdout') if isinstance(row, dict) and isinstance(row.get('stdout'), dict) else row
    if isinstance(obj, dict) and obj.get('comment_id'):
        status_by_comment[str(obj.get('comment_id'))] = obj.get('status') or ''

targets = []
seen = set()
skipped = []

for item in items:
    cid = str(item.get('comment_id') or '')
    tid = str(item.get('thread_id') or '')
    status = status_by_comment.get(cid, '')
    if not tid:
        skipped.append({'comment_id': cid, 'reason': 'missing thread_id', 'status': status})
        continue
    if status not in ('fixed', 'not-confirmed'):
        skipped.append({'comment_id': cid, 'thread_id': tid, 'reason': 'status not resolved automatically', 'status': status})
        continue
    if tid in seen:
        continue
    seen.add(tid)
    targets.append({'comment_id': cid, 'thread_id': tid, 'status': status})

mutation = 'mutation($threadId:ID!){resolveReviewThread(input:{threadId:$threadId}){thread{id isResolved}}}'
resolved = []
failed = []

for target in targets:
    last_err = ''
    for attempt in range(1, 4):
        p = subprocess.run(['gh', 'api', 'graphql', '-f', 'threadId=' + target['thread_id'], '-f', 'query=' + mutation], text=True, capture_output=True)
        if p.returncode == 0:
            resolved.append({**target, 'attempts': attempt})
            break
        last_err = (p.stderr or p.stdout or 'resolveReviewThread failed').strip()
        if any(token in last_err.lower() for token in ('eof', 'timeout', 'tls', 'connection', 'network')) and attempt < 3:
            import time; time.sleep(attempt)
            continue
    else:
        failed.append({**target, 'error': last_err or 'resolveReviewThread failed', 'attempts': 3})

emit({'requested': bool(targets), 'resolved_count': len(resolved), 'failed_count': len(failed), 'resolved': resolved, 'failed': failed, 'skipped': skipped, 'ok': len(failed) == 0})
