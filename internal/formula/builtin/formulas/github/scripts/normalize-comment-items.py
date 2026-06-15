#!/usr/bin/env python3
"""整理 comment 列表供循环使用"""
import json, os, re

comments = json.loads(os.environ.get('TT_COMMENTS') or '{}')
worktree = json.loads(os.environ.get('TT_WORKTREE') or '{}')

items = []
for c in comments.get('comment_items') or []:
    loc = ''
    if c.get('path'):
        loc = c['path'] + ((':' + str(c.get('line'))) if c.get('line') else '')
    summary = (
        f"GitHub PR review comment {c.get('comment_id') or ''}\\n"
        f"位置: {loc or '未提供'}\\n"
        f"作者: {c.get('author') or 'unknown'}\\n"
        f"URL: {c.get('url') or ''}\\n"
        f"关注点: {os.environ.get('TT_REVIEW_FOCUS','')}\\n\\n"
        f"评论内容:\\n{c.get('body') or ''}\\n"
    )
    if c.get('diff_hunk'):
        summary += f"\\n相关 diff hunk:\\n{c.get('diff_hunk')}\\n"
    summary += "\\n请在 PR head 分支对应 worktree 中确认问题是否真实存在，若存在则做最小范围修复并执行必要验证。"
    item = dict(c)
    item['issue_summary'] = summary
    items.append(item)

print(json.dumps({'items': items, 'total': len(items), 'should_process': (len(items) != 0 and bool(worktree.get('ok'))), 'workspace_path': worktree.get('workspace_path') or '', 'worktree_ok': bool(worktree.get('ok')), 'worktree': worktree}, ensure_ascii=False, separators=(',', ':')))
