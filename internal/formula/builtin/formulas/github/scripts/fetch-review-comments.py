#!/usr/bin/env python3
"""获取未解决 review comments"""
import json, os, subprocess, sys

def emit(obj):
    print(json.dumps(obj, ensure_ascii=False, separators=(',', ':')))

def run(cmd):
    return subprocess.run(cmd, text=True, capture_output=True)

meta = json.loads(os.environ.get('TT_PR_META') or '{}')
repo_hint = os.environ.get('TT_REPO_HINT', '').strip()

if not meta.get('ok'):
    emit({'ok': False, 'error': 'pr metadata unavailable', 'comment_items': [], 'pending_count': 0, 'total_threads': 0})
    raise SystemExit(0)

number = int(meta.get('number') or 0)
repo = repo_hint

if not repo:
    p = run(['gh', 'repo', 'view', '--json', 'owner,name'])
    if p.returncode != 0:
        emit({'ok': False, 'error': (p.stderr or p.stdout or 'gh repo view failed').strip(), 'comment_items': [], 'pending_count': 0, 'total_threads': 0})
        raise SystemExit(0)
    rv = json.loads(p.stdout or '{}')
    owner = (rv.get('owner') or {}).get('login') or rv.get('owner') or ''
    name = rv.get('name') or ''
else:
    owner, name = repo.split('/', 1)

query = r'''
query($owner:String!, $name:String!, $number:Int!, $cursor:String) {
  repository(owner:$owner, name:$name) {
    pullRequest(number:$number) {
      number title url headRefName baseRefName state
      reviewThreads(first:100, after:$cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id isResolved isOutdated path line startLine diffSide
          comments(first:50) {
            nodes { id databaseId author { login } body url path line originalLine diffHunk createdAt updatedAt }
          }
        }
      }
    }
  }
}
'''

items=[]; total_threads=0; cursor=''; page=0
while True:
    page += 1
    cmd=['gh','api','graphql','-f',f'owner={owner}','-f',f'name={name}','-F',f'number={number}','-f',f'query={query}']
    if cursor:
        cmd += ['-f', f'cursor={cursor}']
    last_err = ''
    for attempt in range(1, 4):
        p=run(cmd)
        if p.returncode == 0:
            break
        last_err = (p.stderr or p.stdout or 'gh api graphql failed').strip()
        if any(token in last_err.lower() for token in ('eof', 'timeout', 'tls', 'connection', 'network')) and attempt < 3:
            import time; time.sleep(attempt)
            continue
        emit({'ok': False, 'error': last_err, 'attempts': attempt, 'comment_items': items, 'pending_count': len(items), 'total_threads': total_threads})
        raise SystemExit(0)
    else:
        emit({'ok': False, 'error': last_err or 'gh api graphql failed', 'attempts': 3, 'comment_items': items, 'pending_count': len(items), 'total_threads': total_threads})
        raise SystemExit(0)
    data=json.loads(p.stdout or '{}')
    pr=((data.get('data') or {}).get('repository') or {}).get('pullRequest') or {}
    threads=(pr.get('reviewThreads') or {})
    for th in threads.get('nodes') or []:
        total_threads += 1
        if th.get('isResolved'):
            continue
        comments=((th.get('comments') or {}).get('nodes') or [])
        for c in comments:
            body=(c.get('body') or '').strip()
            if not body:
                continue
            items.append({
                'comment_id': c.get('id') or str(c.get('databaseId') or ''),
                'thread_id': th.get('id') or '',
                'author': ((c.get('author') or {}).get('login') or ''),
                'body': body,
                'path': c.get('path') or th.get('path') or '',
                'line': c.get('line') or th.get('line') or c.get('originalLine') or '',
                'url': c.get('url') or '',
                'is_outdated': bool(th.get('isOutdated')),
                'diff_hunk': c.get('diffHunk') or '',
                'created_at': c.get('createdAt') or '',
                'updated_at': c.get('updatedAt') or '',
            })
    pi=threads.get('pageInfo') or {}
    if not pi.get('hasNextPage'):
        break
    cursor=pi.get('endCursor') or ''
    if page >= 20:
        break

emit({'ok': True, 'error': '', 'pr_number': number, 'pr_url': meta.get('url') or '', 'head_branch': meta.get('headRefName') or '', 'base_branch': meta.get('baseRefName') or '', 'repo': f'{owner}/{name}', 'pending_count': len(items), 'total_threads': total_threads, 'comment_items': items})
