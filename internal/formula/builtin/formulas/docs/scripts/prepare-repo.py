#!/usr/bin/env python3
"""准备仓库并抽取基础信息"""
import json, os, re, shutil, subprocess, sys
from pathlib import Path

repo = os.environ.get('TT_REPO', '').strip()
focus_path = os.environ.get('TT_FOCUS_PATH', '').strip()
base = Path(os.getcwd()) / '.tt' / 'tmp' / 'code-docs'
base.mkdir(parents=True, exist_ok=True)

def slugify(value):
    value = re.sub(r'^https?://github.com/', '', value.strip())
    value = re.sub(r'\.git$', '', value)
    value = value.strip('/').replace('/', '-')
    value = re.sub(r'[^A-Za-z0-9._-]+', '-', value).strip('-').lower()
    return value or 'repo'

def run(cmd, cwd=None):
    return subprocess.run(cmd, cwd=cwd, text=True, capture_output=True)

def emit(**kw):
    base_obj = {'ok': False, 'repo_input': repo, 'focus_path': focus_path, 'repo_slug': slugify(repo), 'repo_path': '', 'analysis_path': '', 'source_type': '', 'default_branch': '', 'remote_url': '', 'readme_excerpt': '', 'tree_excerpt': '', 'key_files': [], 'error': ''}
    base_obj.update(kw)
    print(json.dumps(base_obj, ensure_ascii=False, separators=(',', ':')))

if not repo:
    emit(error='missing repo')
    raise SystemExit(0)

candidate = Path(repo).expanduser()
if candidate.exists() and (candidate.is_dir() or candidate.is_file()):
    local_path = candidate.resolve()
    repo_path = local_path if local_path.is_dir() else local_path.parent
    source_type = 'local_file' if local_path.is_file() else 'local_dir'
else:
    source_type = 'github'
    clone_url = repo
    if not repo.startswith(('http://', 'https://', 'git@')):
        clone_url = 'https://github.com/' + repo.strip('/') + '.git'
    repo_path = (base / slugify(repo)).resolve()
    if not (repo_path / '.git').exists():
        if repo_path.exists():
            shutil.rmtree(repo_path)
        p = run(['git', 'clone', '--depth', '1', clone_url, str(repo_path)])
        if p.returncode != 0:
            emit(source_type=source_type, repo_path=str(repo_path), remote_url=clone_url, error=p.stderr.strip() or p.stdout.strip() or 'git clone failed')
            raise SystemExit(0)

remote = run(['git', 'config', '--get', 'remote.origin.url'], cwd=repo_path)
branch = run(['git', 'rev-parse', '--abbrev-ref', 'HEAD'], cwd=repo_path)
analysis_path = local_path if 'local_path' in globals() and local_path.is_file() else repo_path
if focus_path and focus_path != '-' and not analysis_path.is_file():
    maybe = (repo_path / focus_path).resolve()
    try:
        maybe.relative_to(repo_path)
    except Exception:
        emit(source_type=source_type, repo_path=str(repo_path), analysis_path=str(repo_path), error='focus_path escapes repo')
        raise SystemExit(0)
    if not maybe.exists():
        emit(source_type=source_type, repo_path=str(repo_path), analysis_path=str(maybe), error='focus_path not found')
        raise SystemExit(0)
    analysis_path = maybe
readme = ''
for base_dir in ([analysis_path.parent, repo_path] if analysis_path.is_file() else [analysis_path, repo_path]):
    for name in ['README.md', 'README.MD', 'Readme.md', 'README', 'readme.md']:
        path = base_dir / name
        if path.exists() and path.is_file():
            readme = path.read_text(errors='ignore')[:6000]
            break
    if readme:
        break
ignore_dirs = {'.git', 'node_modules', 'dist', 'build', '.next', 'coverage', '.tt', 'vendor', 'target'}
files = []
walk_root = analysis_path if analysis_path.is_dir() else analysis_path.parent
for root, dirs, filenames in os.walk(walk_root):
    rel_root = Path(root).relative_to(repo_path)
    dirs[:] = [d for d in dirs if d not in ignore_dirs and not d.startswith('.')]
    if analysis_path.is_file():
        filenames = [analysis_path.name] if Path(root).resolve() == analysis_path.parent else []
    for filename in filenames:
        rel = rel_root / filename
        rel_s = str(rel).replace('\\\\', '/')
        if rel_s == '.':
            rel_s = filename
        if len(files) < 260:
            files.append(rel_s)
    if len(files) >= 260:
        break
key_patterns = re.compile(r'(^README|package.json$|go.mod$|pyproject.toml$|Cargo.toml$|pom.xml$|build.gradle$|Dockerfile$|docker-compose|Makefile$|tsconfig|vite.config|next.config|src/|cmd/|internal/|app/|pages/|routes/|config)', re.I)
key_files = [f for f in files if key_patterns.search(f)][:80]
tree_excerpt = '\n'.join(files[:220])
repo_slug_value = slugify(repo)
if source_type.startswith('local'):
    if 'local_path' in globals() and local_path.is_file():
        repo_slug_value = slugify(local_path.stem)
    elif repo_slug_value in {'.', '..', ''}:
        repo_slug_value = slugify(repo_path.name)
emit(ok=True, source_type=source_type, repo_slug=repo_slug_value, repo_path=str(repo_path), analysis_path=str(analysis_path), focus_path=focus_path, default_branch=(branch.stdout or '').strip(), remote_url=(remote.stdout or '').strip(), readme_excerpt=readme, tree_excerpt=tree_excerpt, key_files=key_files)
