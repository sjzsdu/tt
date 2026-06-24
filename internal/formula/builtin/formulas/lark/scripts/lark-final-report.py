#!/usr/bin/env python3
import json
import os
from pathlib import Path


def summary_path() -> Path | None:
    explicit = os.environ.get("TT_LARK_RUN_SUMMARY", "").strip()
    if explicit:
        return Path(explicit)
    run_dir = os.environ.get("TT_FORMULA_RUN_DIR", "").strip()
    if run_dir:
        return Path(run_dir) / "lark-auto-reply-summary.jsonl"
    return None


def append_summary(record: dict) -> None:
    path = summary_path()
    if path is None:
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as f:
        f.write(json.dumps(record, ensure_ascii=False, separators=(",", ":")) + "\n")


def load_summary() -> list[dict]:
    path = summary_path()
    if path is None or not path.exists():
        return []
    out = []
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            value = json.loads(line)
            if isinstance(value, dict):
                out.append(value)
        except Exception:
            pass
    return out


def cell(value, limit=120):
    if value is None:
        return ""
    if isinstance(value, (list, dict)):
        value = json.dumps(value, ensure_ascii=False)
    value = str(value).replace("\n", "<br>").replace("|", "\\|")
    if len(value) > limit:
        value = value[: limit - 1] + "…"
    return value


def main():
    records = load_summary()
    print("## Lark 自动回复处理报告\n")
    if not records:
        print("本轮没有处理任何新的 Lark 消息。")
        return
    sent = sum(1 for r in records if r.get("sent") is True)
    rejected = sum(1 for r in records if r.get("status") == "gate_rejected")
    failed = sum(1 for r in records if r.get("status") == "send_failed")
    dry = sum(1 for r in records if r.get("dry_run") is True)
    print(f"本轮共处理 {len(records)} 条消息：发送 {sent} 条，门控拒绝 {rejected} 条，发送失败 {failed} 条，dry-run {dry} 条。\n")
    print("| # | message_id | sender | 原消息 | 处理 | 是否发送 | 回复/原因 |")
    print("|---:|---|---|---|---|---|---|")
    for i, r in enumerate(records, 1):
        status = r.get("status") or "processed"
        sent_text = "是" if r.get("sent") is True else "否"
        detail = r.get("reply") or r.get("reason") or r.get("error") or r.get("note") or ""
        print(f"| {i} | {cell(r.get('message_id'), 40)} | {cell(r.get('sender_name') or r.get('sender_id'), 40)} | {cell(r.get('text'), 160)} | {cell(status, 40)} | {sent_text} | {cell(detail, 260)} |")


if __name__ == "__main__":
    main()
