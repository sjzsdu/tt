#!/usr/bin/env python3
import json
import os
import subprocess
import sys
import time
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any, Dict, Iterable, List


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


def message_timestamp(msg: Dict[str, Any]) -> float:
    raw = str(msg.get("create_time", "")).strip()
    if raw == "":
        return 0.0
    try:
        value = float(raw)
        # Lark timestamps are often milliseconds, sometimes seconds.
        if value > 10_000_000_000:
            value = value / 1000.0
        return value
    except Exception:
        pass
    try:
        return datetime.fromisoformat(raw.replace("Z", "+00:00")).timestamp()
    except Exception:
        return 0.0


def message_sort_key(msg: Dict[str, Any]) -> tuple[float, str]:
    return (message_timestamp(msg), str(msg.get("message_id", "")))


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


def summarize_error(raw: str) -> str:
    raw = (raw or "").strip()
    if raw == "":
        return "lark-cli failed"
    try:
        payload = json.loads(raw)
        if isinstance(payload, dict):
            err = payload.get("error") if isinstance(payload.get("error"), dict) else {}
            code = err.get("code") or payload.get("code")
            message = err.get("message") or payload.get("message") or raw
            typ = err.get("type") or payload.get("type")
            parts = []
            if typ:
                parts.append(str(typ))
            if code:
                parts.append(f"code {code}")
            parts.append(str(message))
            return ": ".join(parts)
    except Exception:
        pass
    return raw[:500]


def run_lark(args: List[str]) -> Dict[str, Any]:
    last_error = ""
    attempts = 3
    for attempt in range(1, attempts + 1):
        proc = subprocess.run(args, text=True, capture_output=True)
        raw = (proc.stdout or "").strip()
        err_raw = (proc.stderr or "").strip()
        if proc.returncode == 0:
            if raw == "":
                return {}
            try:
                payload = json.loads(raw)
            except json.JSONDecodeError as exc:
                raise RuntimeError(f"lark-cli returned non-JSON output: {raw[:500]}") from exc
            if isinstance(payload, dict) and payload.get("ok") is False:
                last_error = summarize_error(raw)
            else:
                return payload
        else:
            last_error = summarize_error(err_raw or raw)
        if attempt < attempts:
            time.sleep(0.8 * attempt)
    raise RuntimeError(f"{last_error} after {attempts} attempts")


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
    sender_type = get_first(raw, ["sender.sender_type", "sender.type", "sender_type"])
    msg_type = get_first(raw, ["msg_type", "message_type", "message.msg_type"])
    chat_type = get_first(raw, ["chat_type", "chat.type", "message.chat_type"])
    mentions = raw.get("mentions") if isinstance(raw.get("mentions"), list) else []
    text = extract_text(raw)
    return {
        "message_id": str(message_id or ""),
        "chat_id": str(chat_id or ""),
        "create_time": str(create_time or ""),
        "sender_id": str(sender_id or ""),
        "sender_name": str(sender_name or ""),
        "sender_type": str(sender_type or ""),
        "msg_type": str(msg_type or ""),
        "chat_type": str(chat_type or ""),
        "mentions": mentions,
        "text": text,
        "source": source,
        "raw": raw,
    }


def is_self_message(msg: Dict[str, Any], self_open_id: str) -> bool:
    sender_type = str(msg.get("sender_type", "")).lower()
    if sender_type in {"app", "bot"}:
        return True
    sender_id = msg.get("sender_id")
    if self_open_id != "" and sender_id == self_open_id:
        return True
    mentions = msg.get("mentions") if isinstance(msg.get("mentions"), list) else []
    for mention in mentions:
        if not isinstance(mention, dict):
            continue
        if sender_id and mention.get("id") == sender_id:
            return True
    return False


def keyword_match(text: str, keywords: List[str]) -> bool:
    if not keywords:
        return True
    low = text.lower()
    return any(k.lower() in low for k in keywords)


def pick_message_group(candidates: List[Dict[str, Any]], group_window_seconds: int) -> List[Dict[str, Any]]:
    if not candidates:
        return []
    picked = candidates[0]
    chat_id = picked.get("chat_id", "")
    if not chat_id or group_window_seconds <= 0:
        return [picked]
    group = [picked]
    last_ts = message_timestamp(picked)
    if last_ts <= 0:
        return group
    for msg in candidates[1:]:
        if msg.get("chat_id", "") != chat_id:
            continue
        ts = message_timestamp(msg)
        if ts <= 0 or ts - last_ts > group_window_seconds:
            break
        group.append(msg)
        if ts > 0:
            last_ts = ts
    return group


def group_text(messages: List[Dict[str, Any]]) -> str:
    if len(messages) <= 1:
        return messages[0].get("text", "") if messages else ""
    lines = []
    for i, msg in enumerate(messages, 1):
        sender = msg.get("sender_name") or msg.get("sender_id") or "unknown"
        text = msg.get("text", "")
        lines.append(f"[{i}] {sender}: {text}")
    return "\n".join(lines)


def stop_requested() -> bool:
    explicit = env("TT_FORMULA_STOP_FILE", "")
    run_dir = env("TT_FORMULA_RUN_DIR", "")
    if explicit and Path(explicit).exists():
        return True
    if run_dir and (Path(run_dir) / "stop-requested").exists():
        return True
    return False


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
    group_window_seconds = max(0, parse_int(env("TT_GROUP_WINDOW_SECONDS", "180"), 180))

    if stop_requested():
        emit({
            "has_message": True,
            "stop_requested": True,
            "reason": "stop requested",
            "fetched_count": 0,
            "candidate_count": 0,
            "errors": [],
            "warnings": ["stop requested"],
        })
        return

    end = now_local()
    start = end - timedelta(minutes=max(1, lookback_minutes))
    state = load_state(state_file)
    processed = state.get("processed", {}) if isinstance(state.get("processed"), dict) else {}
    warnings: List[str] = []

    commands: List[List[str]] = []
    base = [lark_cli, "im", "+messages-search", "--as", identity, "--format", "json", "--page-size", page_size, "--start", iso(start), "--end", iso(end)]
    at_me = base + ["--is-at-me"]
    if chat_ids:
        at_me += ["--chat-id", ",".join(chat_ids)]
    commands.append(at_me)
    if include_direct and chat_ids:
        p2p = base + ["--chat-type", "p2p", "--chat-id", ",".join(chat_ids)]
        commands.append(p2p)
    elif include_direct:
        warnings.append("skip P2P search because chat_ids is empty; broad P2P message search can time out on Lark server")

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
    candidates.sort(key=message_sort_key)
    group = pick_message_group(candidates, group_window_seconds)
    picked = group[-1] if group else None
    if not picked:
        emit({
            "has_message": False,
            "reason": "no relevant unprocessed message",
            "fetched_count": len(messages),
            "candidate_count": 0,
            "errors": errors,
            "warnings": warnings,
            "window": {"start": iso(start), "end": iso(end)},
        })
        return

    emit({
        "has_message": True,
        "message": picked,
        "message_id": picked["message_id"],
        "message_ids": [m.get("message_id", "") for m in group if m.get("message_id", "")],
        "message_group": group,
        "group_size": len(group),
        "group_window_seconds": group_window_seconds,
        "chat_id": picked["chat_id"],
        "source": picked["source"],
        "text": group_text(group),
        "sender_name": picked["sender_name"],
        "sender_id": picked["sender_id"],
        "fetched_count": len(messages),
        "candidate_count": len(candidates),
        "errors": errors,
        "warnings": warnings,
        "window": {"start": iso(start), "end": iso(end)},
    })


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        emit({"has_message": False, "error": str(exc)})
        sys.exit(1)
