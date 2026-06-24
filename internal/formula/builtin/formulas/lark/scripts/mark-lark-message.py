#!/usr/bin/env python3
import json
import os
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict


def emit(obj: Dict[str, Any]) -> None:
    print(json.dumps(obj, ensure_ascii=False, separators=(",", ":")))


def env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


def load_json_env(name: str) -> Dict[str, Any]:
    raw = os.environ.get(name, "").strip()
    if raw == "":
        return {}
    try:
        value = json.loads(raw)
        return value if isinstance(value, dict) else {}
    except Exception:
        return {}


def load_state(path: Path) -> Dict[str, Any]:
    if not path.exists():
        return {"processed": {}}
    try:
        data = json.loads(path.read_text())
        if not isinstance(data, dict):
            data = {"processed": {}}
    except Exception:
        data = {"processed": {}}
    if not isinstance(data.get("processed"), dict):
        data["processed"] = {}
    return data


def save_state(path: Path, state: Dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(json.dumps(state, ensure_ascii=False, indent=2) + "\n")
    tmp.replace(path)


def append_summary(pick: Dict[str, Any], gate: Dict[str, Any], record: Dict[str, Any]) -> None:
    explicit = env("TT_LARK_RUN_SUMMARY", "")
    run_dir = env("TT_FORMULA_RUN_DIR", "")
    path = Path(explicit) if explicit else (Path(run_dir) / "lark-auto-reply-summary.jsonl" if run_dir else None)
    if path is None:
        return
    message = pick.get("message") if isinstance(pick.get("message"), dict) else {}
    base: Dict[str, Any] = {
        "message_id": pick.get("message_id") or message.get("message_id"),
        "chat_id": pick.get("chat_id") or message.get("chat_id"),
        "source": pick.get("source"),
        "sender_name": pick.get("sender_name") or message.get("sender_name"),
        "sender_id": pick.get("sender_id") or message.get("sender_id"),
        "text": pick.get("text") or message.get("text"),
        "reason": gate.get("reason"),
    }
    base.update(record)
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as f:
        f.write(json.dumps(base, ensure_ascii=False, separators=(",", ":")) + "\n")


def main() -> None:
    state_file = Path(env("TT_STATE_FILE", ".tt/lark-auto-reply/state.json"))
    pick = load_json_env("TT_PICK_MESSAGE")
    gate = load_json_env("TT_REPLY_GATE")
    reason = env("TT_MARK_REASON", "processed")
    message = pick.get("message") if isinstance(pick.get("message"), dict) else {}
    message_id = str(pick.get("message_id") or message.get("message_id") or "")
    if not message_id:
        emit({"marked": False, "reason": "missing message_id"})
        return

    state = load_state(state_file)
    processed = state.setdefault("processed", {})
    if message_id in processed:
        emit({"marked": True, "message_id": message_id, "reason": "already processed"})
        return

    processed_at = datetime.now(timezone.utc).isoformat()
    processed[message_id] = {
        "processed_at": processed_at,
        "mode": reason,
        "chat_id": pick.get("chat_id") or message.get("chat_id"),
        "source": pick.get("source"),
        "sender_id": pick.get("sender_id") or message.get("sender_id"),
        "gate_reason": gate.get("reason"),
        "risk_level": gate.get("risk_level"),
        "needs_human": gate.get("needs_human"),
    }
    state["last_processed_at"] = processed_at
    save_state(state_file, state)
    append_summary(pick, gate, {"status": reason, "sent": False, "marked": True})
    emit({"marked": True, "message_id": message_id, "reason": reason})


if __name__ == "__main__":
    main()
