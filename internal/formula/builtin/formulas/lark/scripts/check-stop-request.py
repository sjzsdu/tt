#!/usr/bin/env python3
import json
import os
from pathlib import Path


def stop_path() -> Path | None:
    explicit = os.environ.get("TT_FORMULA_STOP_FILE", "").strip()
    if explicit:
        return Path(explicit)
    run_dir = os.environ.get("TT_FORMULA_RUN_DIR", "").strip()
    if run_dir:
        return Path(run_dir) / "stop-requested"
    return None


path = stop_path()
should_stop = bool(path and path.exists())
print(json.dumps({"should_stop": should_stop, "stop_file": str(path) if path else ""}, ensure_ascii=False))
