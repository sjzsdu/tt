#!/usr/bin/env python3
"""抽取仓库证据地图"""
import json, os
from pathlib import Path

repo_path = Path(os.environ.get('TT_REPO_PATH','')).resolve()
analysis_path = Path(os.environ.get('TT_ANALYSIS_PATH','') or repo_path).resolve()

if not repo_path.exists() or not analysis_path.exists():
    print(json.dumps({'ok':False,'error':'repo_path or analysis_path not found','repo_path':str(repo_path),'analysis_path':str(analysis_path)}, ensure_ascii=False, separators=(',', ':')))
    raise SystemExit(0)

ignore_dirs = {'.git','node_modules','dist','build','.next','coverage','.tt','vendor','target','__pycache__','.venv','venv','out','.turbo'}
text_ext = {'.ts','.tsx','.js','.jsx','.go','.py','.rs','.java','.kt','.swift','.md','.mdx','.json','.toml','.yaml','.yml','.graphql','.proto','.sql','.css','.scss','.html','.sh','.rb','.php'}
important_names = {'README.md','package.json','go.mod','pyproject.toml','Cargo.toml','Makefile','Dockerfile','docker-compose.yml','tsconfig.json','vite.config.ts','next.config.js','requirements.txt','pom.xml','build.gradle'}
entry_words = ['main','index','app','server','cli','route','router','handler','controller','command','plugin','extension']
model_words = ['model','schema','entity','types','interface','store','state','migration']
flow_words = ['workflow','pipeline','service','manager','engine','runner','processor','scheduler']

items = []
existing_docs = []
stats = {'files_seen':0,'source_files':0,'test_files':0,'config_files':0,'doc_files':0,'dirs':set(),'extensions':{}}

if analysis_path.is_file():
    walk_files = [(analysis_path.parent, [], [analysis_path.name])]
else:
    walk_files = os.walk(analysis_path)

for root_dir, dirs, files in walk_files:
    root_dir = Path(root_dir)
    rel_root = root_dir.relative_to(repo_path)
    dirs[:] = [d for d in dirs if d not in ignore_dirs and not d.startswith('.')]
    stats['dirs'].add(str(rel_root).replace('\\','/'))
    for fn in files:
        path = root_dir / fn
        if not path.exists() or path.is_dir():
            continue
        rel = str((rel_root / fn)).replace('\\','/')
        ext = path.suffix.lower()
        stats['files_seen'] += 1
        stats['extensions'][ext or '<none>'] = stats['extensions'].get(ext or '<none>',0)+1
        lower = rel.lower()
        if ext in {'.ts','.tsx','.js','.jsx','.go','.py','.rs','.java','.kt','.swift','.rb','.php'}:
            stats['source_files'] += 1
        if any(x in lower for x in ['test','spec','__tests__']):
            stats['test_files'] += 1
        if fn in important_names or ext in {'.toml','.yaml','.yml','.json'}:
            stats['config_files'] += 1
            score_config = True
        else:
            score_config = False
        if ext in {'.md','.mdx'}:
            stats['doc_files'] += 1
            if lower.startswith(('ai-docs/','docs/')) or 'readme' in lower:
                try:
                    existing_docs.append({'path':rel,'excerpt':path.read_text(errors='ignore')[:1800]})
                except Exception:
                    existing_docs.append({'path':rel,'excerpt':''})
        if ext not in text_ext:
            continue
        score = 0
        name_l = fn.lower()
        if fn in important_names or name_l.startswith('readme'):
            score += 8
        if score_config:
            score += 3
        if any(w in lower for w in entry_words):
            score += 5
        if any(w in lower for w in flow_words):
            score += 4
        if any(w in lower for w in model_words):
            score += 4
        if rel.startswith(('src/','app/','pages/','routes/','cmd/','internal/','pkg/','lib/','server/','frontend/','backend/','plugins/')):
            score += 3
        if any(x in lower for x in ['test','spec','example','demo']):
            score += 2
        if score <= 0:
            continue
        try:
            text = path.read_text(errors='ignore')[:3600]
        except Exception:
            text = ''
        items.append({'path': rel, 'score': score, 'kind': ('doc' if ext in {'.md','.mdx'} else 'code'), 'excerpt': text})

items.sort(key=lambda x: (-x['score'], x['path']))
extensions = sorted(stats['extensions'].items(), key=lambda kv:(-kv[1], kv[0]))[:20]
stats_out = dict(stats)
stats_out['dirs'] = sorted([d for d in stats['dirs'] if d and d != '.'])[:160]
stats_out['extensions'] = [{'ext':k,'count':v} for k,v in extensions]

complexity = 'small'
if stats['source_files'] > 120 or len(stats['dirs']) > 60:
    complexity = 'large'
elif stats['source_files'] > 30 or len(stats['dirs']) > 20:
    complexity = 'medium'

print(json.dumps({'ok':True,'repo_path':str(repo_path),'analysis_path':str(analysis_path),'complexity':complexity,'stats':stats_out,'existing_docs':existing_docs[:30],'evidence_files':items[:90]}, ensure_ascii=False, separators=(',', ':')))
