#!/usr/bin/env python3
import json
import os
import re
import subprocess
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional


def emit(obj: Dict[str, Any]) -> None:
    print(json.dumps(obj, ensure_ascii=False, separators=(",", ":")))


def env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


def parse_bool(value: str, default: bool = False) -> bool:
    value = value.strip().lower()
    if value == "":
        return default
    return value in {"1", "true", "yes", "y", "on"}


def parse_int(value: str, default: int) -> int:
    try:
        return int(value)
    except Exception:
        return default


def split_csv(value: str) -> List[str]:
    return [part.strip() for part in value.split(",") if part.strip()]


def now_local() -> datetime:
    return datetime.now().astimezone()


def iso(dt: datetime) -> str:
    return dt.isoformat(timespec="seconds")


def load_state(path: Path) -> Dict[str, Any]:
    if not path.exists():
        return {"processed": {}}
    try:
        data = json.loads(path.read_text())
        if not isinstance(data, dict):
            return {"processed": {}}
        if not isinstance(data.get("processed"), dict):
            data["processed"] = {}
        return data
    except Exception:
        return {"processed": {}}


def run_lark(args: List[str]) -> Dict[str, Any]:
    proc = subprocess.run(args, text=True, capture_output=True)
    if proc.returncode != 0:
        raise RuntimeError((proc.stderr or proc.stdout or "lark-cli failed").strip())
    raw = proc.stdout.strip()
    if raw == "":
        return {}
    try:
        return json.loads(raw)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"lark-cli returned non-JSON output: {raw[:500]}") from exc


def find_items(value: Any) -> List[Dict[str, Any]]:
    if isinstance(value, list):
        return [v for v in value if isinstance(v, dict)]
    if not isinstance(value, dict):
        return []
    for key in ("items", "messages", "data", "results"):
        child = value.get(key)
        if isinstance(child, list):
            return [v for v in child if isinstance(v, dict)]
        nested = find_items(child)
        if nested:
            return nested
    return []


def get_first(obj: Dict[str, Any], paths: Iterable[str]) -> Any:
    for path in paths:
        cur: Any = obj
        ok = True
        for part in path.split("."):
            if isinstance(cur, dict) and part in cur:
                cur = cur[part]
            else:
                ok = False
                break
        if ok and cur not in (None, ""):
            return cur
    return None


def extract_text(message: Dict[str, Any]) -> str:
    content = get_first(message, ["content", "body.content", "message.content"])
    if isinstance(content, dict):
        # Lark text content commonly looks like {"text":"..."}; post content is nested.
        if isinstance(content.get("text"), str):
            return content["text"]
        return json.dumps(content, ensure_ascii=False)
    if not isinstance(content, str):
        fallback = get_first(message, ["text", "body.text", "message.text"])
        return fallback if isinstance(fallback, str) else ""
    text = content
    try:
        parsed = json.loads(content)
        if isinstance(parsed, dict):
            if isinstance(parsed.get("text"), str):
                text = parsed["text"]
            else:
                text = json.dumps(parsed, ensure_ascii=False)
    except Exception:
        pass
    return text.strip()


def normalize_message(raw: Dict[str, Any], source: str) -> Dict[str, Any]:
    message_id = get_first(raw, ["message_id", "message.message_id", "id", "message.id"])
    chat_id = get_first(raw, ["chat_id", "message.chat_id", "chat.id"])
    create_time = get_first(raw, ["create_time", "message.create_time", "create_time_ms", "message.create_time_ms"])
    sender_id = get_first(raw, ["sender.id", "sender.sender_id.open_id", "sender.open_id", "sender_id.open_id", "sender_id"])
    sender_name = get_first(raw, ["sender.name", "sender.sender_name", "sender_name", "sender.display_name"])
    msg_type = get_first(raw, ["msg_type", "message_type", "message.msg_type"])
    chat_type = get_first(raw, ["chat_type", "chat.type", "message.chat_type"])
    text = extract_text(raw)
    return {
        "message_id": str(message_id or ""),
        "chat_id": str(chat_id or ""),
        "create_time": str(create_time or ""),
        "sender_id": str(sender_id or ""),
        "sender_name": str(sender_name or ""),
        "msg_type": str(msg_type or ""),
        "chat_type": str(chat_type or ""),
        "text": text,
        "source": source,
        "raw": raw,
    }


def is_self_message(msg: Dict[str, Any], self_open_id: str) -> bool:
    return self_open_id != "" and msg.get("sender_id") == self_open_id


def keyword_match(text: str, keywords: List[str]) -> bool:
    if not keywords:
        return True
    low = text.lower()
    return any(k.lower() in low for k in keywords)


def main() -> None:
    lark_cli = env("TT_LARK_CLI", "lark-cli")
    identity = env("TT_LARK_AS", "user")
    state_file = Path(env("TT_STATE_FILE", ".tt/lark-auto-reply/state.json"))
    self_open_id = env("TT_SELF_OPEN_ID", "")
    chat_ids = split_csv(env("TT_CHAT_IDS", ""))
    keywords = split_csv(env("TT_KEYWORDS", ""))
    include_direct = parse_bool(env("TT_INCLUDE_DIRECT", "true"), True)
    lookback_minutes = parse_int(env("TT_LOOKBACK_MINUTES", "5"), 5)
    page_size = str(max(1, min(parse_int(env("TT_PAGE_SIZE", "20"), 20), 50)))

    end = now_local()
    start = end - timedelta(minutes=max(1, lookback_minutes))
    state = load_state(state_file)
    processed = state.get("processed", {}) if isinstance(state.get("processed"), dict) else {}

    commands: List[List[str]] = []
    base = [lark_cli, "im", "+messages-search", "--as", identity, "--format", "json", "--page-size", page_size, "--start", iso(start), "--end", iso(end)]
    at_me = base + ["--is-at-me"]
    if chat_ids:
        at_me += ["--chat-id", ",".join(chat_ids)]
    commands.append(at_me)
    if include_direct:
        p2p = base + ["--chat-type", "p2p"]
        if chat_ids:
            p2p += ["--chat-id", ",".join(chat_ids)]
        commands.append(p2p)

    messages: List[Dict[str, Any]] = []
    errors: List[str] = []
    for command in commands:
        try:
            payload = run_lark(command)
            source = "at-me" if "--is-at-me" in command else "p2p"
            messages.extend(normalize_message(item, source) for item in find_items(payload))
        except Exception as exc:
            errors.append(str(exc))

    seen = set()
    candidates = []
    for msg in messages:
        mid = msg.get("message_id", "")
        if mid == "" or mid in seen:
            continue
        seen.add(mid)
        if mid in processed:
            continue
        if is_self_message(msg, self_open_id):
            continue
        if not keyword_match(msg.get("text", ""), keywords):
            continue
        candidates.append(msg)

    # Oldest first to preserve conversational ordering.
    candidates.sort(key=lambda m: m.get("create_time", ""))
    picked = candidates[0] if candidates else None
    if not picked:
        emit({
            "has_message": False,
            "reason": "no relevant unprocessed message",
            "fetched_count": len(messages),
            "candidate_count": 0,
            "errors": errors,
            "window": {"start": iso(start), "end": iso(end)},
        })
        return

    emit({
        "has_message": True,
        "message": picked,
        "message_id": picked["message_id"],
        "chat_id": picked["chat_id"],
        "source": picked["source"],
        "text": picked["text"],
        "sender_name": picked["sender_name"],
        "sender_id": picked["sender_id"],
        "fetched_count": len(messages),
        "candidate_count": len(candidates),
        "errors": errors,
        "window": {"start": iso(start), "end": iso(end)},
    })


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        emit({"has_message": False, "error": str(exc)})
        sys.exit(1)
