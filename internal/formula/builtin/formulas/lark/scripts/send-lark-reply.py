#!/usr/bin/env python3
import hashlib
import json
import os
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List


def emit(obj: Dict[str, Any]) -> None:
    print(json.dumps(obj, ensure_ascii=False, separators=(",", ":")))


def env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


def parse_bool(value: str, default: bool = False) -> bool:
    value = value.strip().lower()
    if value == "":
        return default
    return value in {"1", "true", "yes", "y", "on"}


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


def run_lark(args: List[str]) -> Dict[str, Any]:
    proc = subprocess.run(args, text=True, capture_output=True)
    if proc.returncode != 0:
        raise RuntimeError((proc.stderr or proc.stdout or "lark-cli failed").strip())
    raw = proc.stdout.strip()
    if raw == "":
        return {}
    try:
        payload = json.loads(raw)
    except Exception:
        return {"raw": raw}
    if isinstance(payload, dict) and payload.get("ok") is False:
        raise RuntimeError(raw)
    return payload


def send_error_payload(exc: Exception, message_id: str) -> Dict[str, Any]:
    raw = str(exc).strip()
    payload: Dict[str, Any] = {
        "sent": False,
        "marked": False,
        "message_id": message_id,
        "error": raw,
        "action_required": "manual_fix",
    }
    try:
        parsed = json.loads(raw)
        if isinstance(parsed, dict):
            err = parsed.get("error") if isinstance(parsed.get("error"), dict) else {}
            message = err.get("message") or parsed.get("message")
            typ = err.get("type") or parsed.get("type")
            hint = err.get("hint") or parsed.get("hint")
            if message:
                payload["error_message"] = message
            if typ:
                payload["error_type"] = typ
            if hint:
                payload["hint"] = hint
    except Exception:
        pass
    if "im:message.send_as_user" in raw:
        payload["auth_command"] = "lark-cli auth login --scope \"im:message.send_as_user\""
    return payload


def trim_reply(text: str, max_chars: int) -> str:
    text = text.strip()
    # Strip accidental fenced code wrappers around plain replies.
    if text.startswith("```") and text.endswith("```"):
        lines = text.splitlines()
        if len(lines) >= 3:
            text = "\n".join(lines[1:-1]).strip()
    if max_chars > 0 and len(text) > max_chars:
        return text[: max_chars - 20].rstrip() + "\n…（已截断）"
    return text


def main() -> None:
    lark_cli = env("TT_LARK_CLI", "lark-cli")
    identity = env("TT_LARK_AS", "user")
    mode = env("TT_MODE", "dry-run").lower()
    state_file = Path(env("TT_STATE_FILE", ".tt/lark-auto-reply/state.json"))
    pick = load_json_env("TT_PICK_MESSAGE")
    message = pick.get("message") if isinstance(pick.get("message"), dict) else {}
    message_id = str(pick.get("message_id") or message.get("message_id") or "")
    reply_text = os.environ.get("TT_REPLY_TEXT", "").strip()
    max_chars = int(env("TT_MAX_REPLY_CHARS", "1200") or "1200")
    reply_in_thread = parse_bool(env("TT_REPLY_IN_THREAD", "true"), True)

    if not message_id:
        emit({"sent": False, "marked": False, "reason": "missing message_id"})
        return
    reply_text = trim_reply(reply_text, max_chars)
    if not reply_text:
        emit({"sent": False, "marked": False, "message_id": message_id, "reason": "empty reply"})
        return

    if mode not in {"dry-run", "auto"}:
        raise RuntimeError("TT_MODE must be dry-run or auto")

    state = load_state(state_file)
    processed = state.setdefault("processed", {})
    if message_id in processed:
        emit({"sent": False, "marked": True, "message_id": message_id, "reason": "already processed"})
        return

    if mode == "dry-run":
        emit({
            "sent": False,
            "marked": False,
            "dry_run": True,
            "message_id": message_id,
            "reply": reply_text,
            "note": "dry-run does not write Lark or mark state",
        })
        return

    idem = "tt-lark-auto-reply-" + hashlib.sha256(message_id.encode()).hexdigest()[:24]
    args = [
        lark_cli,
        "im",
        "+messages-reply",
        "--as",
        identity,
        "--message-id",
        message_id,
        "--text",
        reply_text,
        "--idempotency-key",
        idem,
    ]
    if reply_in_thread:
        args.append("--reply-in-thread")
    try:
        response = run_lark(args)
    except RuntimeError as exc:
        emit(send_error_payload(exc, message_id))
        return

    processed[message_id] = {
        "processed_at": datetime.now(timezone.utc).isoformat(),
        "mode": mode,
        "idempotency_key": idem,
        "chat_id": pick.get("chat_id") or message.get("chat_id"),
        "source": pick.get("source"),
    }
    state["last_processed_at"] = processed[message_id]["processed_at"]
    save_state(state_file, state)
    emit({"sent": True, "marked": True, "message_id": message_id, "idempotency_key": idem, "response": response})


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        emit({"sent": False, "marked": False, "error": str(exc)})
        sys.exit(1)
