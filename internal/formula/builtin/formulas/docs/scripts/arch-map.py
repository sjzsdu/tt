#!/usr/bin/env python3
"""抽取架构证据地图：入口、模块边界、依赖边、数据/API/状态/风险线索。"""
import json, os, re
from collections import Counter, defaultdict
from pathlib import Path

repo_path = Path(os.environ.get('TT_REPO_PATH','')).resolve()
analysis_path = Path(os.environ.get('TT_ANALYSIS_PATH','') or repo_path).resolve()

if not repo_path.exists() or not analysis_path.exists():
    print(json.dumps({'ok':False,'error':'repo_path or analysis_path not found','repo_path':str(repo_path),'analysis_path':str(analysis_path)}, ensure_ascii=False, separators=(',', ':')))
    raise SystemExit(0)

ignore_dirs = {'.git','node_modules','dist','build','.next','coverage','.tt','vendor','target','__pycache__','.venv','venv','out','.turbo'}
source_ext = {'.ts','.tsx','.js','.jsx','.mjs','.cjs','.go','.py','.rs','.java','.kt','.swift','.rb','.php','.cs'}
config_names = {'package.json','go.mod','pyproject.toml','Cargo.toml','pom.xml','build.gradle','Dockerfile','docker-compose.yml','tsconfig.json','vite.config.ts','next.config.js','requirements.txt','Makefile'}
entry_patterns = re.compile(r'(^|/)(main|index|app|server|cli|router|routes?|handler|controller|command|worker|bootstrap)\.[^.]+$', re.I)
api_words = re.compile(r'api|route|router|controller|handler|endpoint|graphql|rpc|client', re.I)
state_words = re.compile(r'store|state|reducer|context|atom|signal|machine|workflow|status|cache', re.I)
data_words = re.compile(r'model|schema|entity|dto|codec|serializer|migration|repository|dao|database|db', re.I)
risk_words = re.compile(r'todo|fixme|hack|deprecated|legacy|workaround|temporary|unsafe|any|panic|throw new|console\.error', re.I)

import_re = re.compile(r"(?:import(?:[^'\"]+from)?|export[^'\"]+from|require\()\s*['\"]([^'\"]+)['\"]")
go_import_re = re.compile(r'^\s*"([^"]+)"', re.M)
py_import_re = re.compile(r'^\s*(?:from\s+([a-zA-Z0-9_\.]+)\s+import|import\s+([a-zA-Z0-9_\.]+))', re.M)

files = []
configs = []
entrypoints = []
api_hints = []
state_hints = []
data_hints = []
risk_hints = []
cluster_files = defaultdict(list)
dep_edges = Counter()

if analysis_path.is_file():
    walk_files = [(analysis_path.parent, [], [analysis_path.name])]
else:
    walk_files = os.walk(analysis_path)

def rel_to_repo(path: Path) -> str:
    try:
        return str(path.relative_to(repo_path)).replace('\\','/')
    except Exception:
        return str(path).replace('\\','/')

def cluster_for(rel: str) -> str:
    parts = rel.split('/')
    if len(parts) >= 3 and parts[0] in {'src','app','pages','routes','cmd','internal','pkg','lib','server','frontend','backend'}:
        return '/'.join(parts[:3 if parts[0] in {'src','app','pages'} else 2])
    if len(parts) >= 2:
        return '/'.join(parts[:2])
    return parts[0]

def target_cluster(import_path: str, current_rel: str) -> str:
    if import_path.startswith('.'):
        base = Path(current_rel).parent
        normalized = str((base / import_path).as_posix()).replace('/./','/')
        while '/../' in normalized or normalized.startswith('../'):
            normalized = normalized.replace('/../','/',1)
            if normalized.startswith('../'):
                normalized = normalized[3:]
        return cluster_for(normalized)
    root = import_path.split('/')[0]
    if root.startswith('@') and len(import_path.split('/')) > 1:
        root = '/'.join(import_path.split('/')[:2])
    return f'external:{root}'

def add_hint(bucket, rel, reason, excerpt=''):
    if len(bucket) < 80:
        bucket.append({'path': rel, 'reason': reason, 'excerpt': excerpt[:600]})

for root_dir, dirs, names in walk_files:
    root_dir = Path(root_dir)
    dirs[:] = [d for d in dirs if d not in ignore_dirs and not d.startswith('.')]
    for name in names:
        path = root_dir / name
        if not path.exists() or path.is_dir():
            continue
        rel = rel_to_repo(path)
        ext = path.suffix.lower()
        lower = rel.lower()
        if name in config_names:
            configs.append(rel)
        if ext not in source_ext:
            continue
        files.append(rel)
        cluster = cluster_for(rel)
        cluster_files[cluster].append(rel)
        try:
            text = path.read_text(errors='ignore')
        except Exception:
            text = ''
        excerpt = '\n'.join(text.splitlines()[:24])
        if entry_patterns.search(rel):
            entrypoints.append({'path': rel, 'cluster': cluster, 'reason': 'entrypoint-like filename', 'excerpt': excerpt[:900]})
        if api_words.search(rel) or re.search(r'\b(fetch|axios|useQuery|query|mutation|http|grpc|GraphQL)\b', text):
            add_hint(api_hints, rel, 'API/route/client signal', excerpt)
        if state_words.search(rel) or re.search(r'\b(useState|useReducer|createContext|zustand|redux|machine|status)\b', text):
            add_hint(state_hints, rel, 'state/workflow/cache signal', excerpt)
        if data_words.search(rel) or re.search(r'\b(interface|type|class|struct|schema|codec|serialize|deserialize)\b', text):
            add_hint(data_hints, rel, 'data/model/schema signal', excerpt)
        risk_lines = []
        for i, line in enumerate(text.splitlines()[:500], start=1):
            if risk_words.search(line):
                risk_lines.append(f'{i}: {line.strip()}')
                if len(risk_lines) >= 4:
                    break
        if risk_lines:
            add_hint(risk_hints, rel, 'risk keyword signal', '\n'.join(risk_lines))
        imports = []
        imports.extend(import_re.findall(text[:12000]))
        if ext == '.go':
            imports.extend(go_import_re.findall(text[:12000]))
        if ext == '.py':
            for a,b in py_import_re.findall(text[:12000]):
                imports.append(a or b)
        for imp in imports[:80]:
            dep_edges[(cluster, target_cluster(imp, rel))] += 1

clusters = []
for name, paths in cluster_files.items():
    lower_name = name.lower()
    signals = []
    if api_words.search(lower_name): signals.append('api')
    if state_words.search(lower_name): signals.append('state')
    if data_words.search(lower_name): signals.append('data')
    if any('/components' in p or p.endswith(('component.tsx','component.jsx')) for p in paths): signals.append('ui-components')
    if any('/hooks' in p or '/hook' in p for p in paths): signals.append('hooks')
    if any('test' in p.lower() or 'spec' in p.lower() for p in paths): signals.append('tests')
    clusters.append({'name': name, 'file_count': len(paths), 'sample_files': sorted(paths)[:16], 'signals': signals})
clusters.sort(key=lambda x: (-x['file_count'], x['name']))

edges = []
for (src, dst), count in dep_edges.most_common(120):
    if src == dst:
        continue
    edges.append({'from': src, 'to': dst, 'weight': count})

complexity = 'small'
if len(files) > 150 or len(clusters) > 50:
    complexity = 'large'
elif len(files) > 40 or len(clusters) > 15:
    complexity = 'medium'

out = {
    'ok': True,
    'repo_path': str(repo_path),
    'analysis_path': str(analysis_path),
    'complexity': complexity,
    'source_file_count': len(files),
    'config_files': sorted(configs)[:60],
    'entrypoints': entrypoints[:40],
    'module_clusters': clusters[:80],
    'dependency_hints': edges,
    'api_hints': api_hints,
    'state_hints': state_hints,
    'data_hints': data_hints,
    'risk_hints': risk_hints,
    'diagram_candidates': [
        {'type':'system-context','why':'entrypoints + external dependency hints can reveal system boundary'},
        {'type':'module-boundary','why':'module_clusters and dependency_hints can reveal internal layering'},
        {'type':'data-flow','why':'api_hints + data_hints + state_hints can reveal request/data/state flow'},
        {'type':'risk-hotspots','why':'risk_hints and high-degree dependency clusters can reveal evolution risks'},
    ]
}
print(json.dumps(out, ensure_ascii=False, separators=(',', ':')))
