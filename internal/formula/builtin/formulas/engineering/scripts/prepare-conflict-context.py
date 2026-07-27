#!/usr/bin/env python3
"""准备冲突上下文"""
import hashlib, json, os, pathlib, subprocess, sys, traceback

def run(args):
    p = subprocess.run(args, capture_output=True)
    return p.stdout.decode("utf-8", "replace")

def run_text(args):
    p = subprocess.run(args, capture_output=True)
    return p.returncode, p.stdout.decode("utf-8", "replace"), p.stderr.decode("utf-8", "replace")

def trunc(text, limit=24000):
    if text is None: return ""
    if len(text) <= limit: return text
    return text[:limit] + f"\n...<truncated {len(text) - limit} chars>"

def stage(path, n):
    rc, stdout, stderr = run_text(["git", "show", f":{n}:{path}"])
    if rc != 0:
        return ""
    return stdout

def hunks(path, radius=24, limit=160000):
    try: lines=open(path, encoding="utf-8", errors="replace").read().splitlines()
    except OSError: return ""
    ranges=[]
    for i,line in enumerate(lines):
        if line.startswith("<<<<<<<"):
            start=max(0,i-radius); end=min(len(lines),i+radius); j=i+1
            while j < len(lines):
                if lines[j].startswith(">>>>>>>"):
                    end=min(len(lines),j+radius+1); break
                j+=1
            ranges.append((start,end))
    merged=[]
    for start,end in ranges:
        if merged and start <= merged[-1][1]: merged[-1]=(merged[-1][0], max(merged[-1][1], end))
        else: merged.append((start,end))
    chunks=[]
    for start,end in merged:
        chunks.append("\n".join(f"{idx+1}: {lines[idx]}" for idx in range(start,end)))
    return trunc("\n\n--- conflict hunk ---\n\n".join(chunks), limit)

def conflict_regions(path):
    try: lines=open(path, encoding="utf-8", errors="replace").read().splitlines()
    except OSError: return []
    regions=[]; i=0
    while i < len(lines):
        if lines[i].startswith("<<<<<<<"):
            start=i+1; sep=None; end=None; j=i+1
            while j < len(lines):
                if lines[j].startswith("=======") and sep is None:
                    sep=j+1
                if lines[j].startswith(">>>>>>>"):
                    end=j+1; break
                j+=1
            regions.append({"start_line":start,"separator_line":sep,"end_line":end,"summary":f"lines {start}-{end or '?'}"})
            i=j if end else i+1
        i+=1
    return regions

try:
    requested_repo_root=os.environ.get("TT_REPO_ROOT", "").strip()
    if requested_repo_root:
        os.chdir(requested_repo_root)
    env_context_root=os.environ.get("TT_CONTEXT_ROOT", "").strip()
    repo_root=run(["git","rev-parse","--show-toplevel"]).strip()
    current_branch=run(["git","branch","--show-current"]).strip()
    merge_head=run(["git","rev-parse","--git-path","MERGE_HEAD"]).strip()
    rebase_merge=run(["git","rev-parse","--git-path","rebase-merge"]).strip()
    rebase_apply=run(["git","rev-parse","--git-path","rebase-apply"]).strip()
    cherry_pick=run(["git","rev-parse","--git-path","CHERRY_PICK_HEAD"]).strip()
    operation="none"
    if merge_head and os.path.exists(merge_head): operation="merge"
    if (rebase_merge and os.path.isdir(rebase_merge)) or (rebase_apply and os.path.isdir(rebase_apply)): operation="rebase"
    if cherry_pick and os.path.exists(cherry_pick): operation="cherry-pick"

    unmerged_files=[p for p in run(["git","diff","--name-only","--diff-filter=U"]).splitlines() if p]
    marker_files=[]
    for candidate in [p for p in run(["git","diff","--name-only","HEAD"]).splitlines() if p]:
        if os.path.isfile(candidate):
            grep=subprocess.run(["git","grep","-n","-E",r"^(<<<<<<<|=======|>>>>>>>)","--",candidate], capture_output=True)
            if grep.returncode == 0:
                marker_files.append(candidate)
    files=sorted(set(unmerged_files + marker_files))
    unmerged_set=set(unmerged_files)
    ctx_base=pathlib.Path(env_context_root) if env_context_root else pathlib.Path(".")
    ctx_dir=ctx_base / ".tt/conflict-context/git-resolve-conflicts"
    ctx_dir.mkdir(parents=True, exist_ok=True)
    items=[]
    for path in files:
        if path in unmerged_set:
            ours=stage(path,2); theirs=stage(path,3); base=stage(path,1)
            context_note="ours is the current side; theirs is the incoming side; base is the merge base. Preserve compatible intent from both sides."
        else:
            ours=theirs=base=""
            context_note="This file has conflict markers in the working tree but no unmerged index stages. Resolve markers conservatively from the file content and nearby project context."
        key=hashlib.sha1(path.encode()).hexdigest()[:12]
        context_file=str(ctx_dir / f"{key}.json")
        regions=conflict_regions(path)
        payload={"path":path,"context_note":context_note,"conflict_regions":regions,"base_excerpt":trunc(base,40000),"ours_excerpt":trunc(ours,80000),"theirs_excerpt":trunc(theirs,80000),"conflict_hunks":hunks(path),"ours_stat":{"chars":len(ours),"lines":ours.count("\n")+(1 if ours else 0)},"theirs_stat":{"chars":len(theirs),"lines":theirs.count("\n")+(1 if theirs else 0)}}
        pathlib.Path(context_file).write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")
        items.append({"path":path,"file_kind":os.path.splitext(path)[1].lstrip('.') or os.path.basename(path),"context_file":context_file,"conflict_regions":regions,"conflict_region_count":len(regions),"ours_stat":payload["ours_stat"],"theirs_stat":payload["theirs_stat"]})
    started_at=os.environ.get("TT_STARTED_AT", "")
    started_epoch=int(os.environ.get("TT_STARTED_EPOCH", "0") or "0")
    print(json.dumps({"ok":True,"repo_root":repo_root,"current_branch":current_branch,"operation":operation,"started_at":started_at,"started_epoch":started_epoch,"needs_resolution":len(files)>0,"done":len(files)==0,"files":files,"items":items,"conflicted_files":files,"unmerged_files":unmerged_files,"marker_files":sorted(set(marker_files)),"blocker_type":"none" if files else "none","blocker_summary":"" if files else "no current git conflict detected"}, ensure_ascii=False))
except Exception as exc:
    print(json.dumps({"ok":False,"needs_resolution":False,"done":False,"files":[],"items":[],"error":str(exc),"traceback":traceback.format_exc(limit=6)}, ensure_ascii=False))
    sys.exit(1)
