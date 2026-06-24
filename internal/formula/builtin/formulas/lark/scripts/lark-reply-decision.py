#!/usr/bin/env python3
import json
import os


def load(name):
    raw = os.environ.get(name, "").strip()
    if not raw:
        return {}
    try:
        value = json.loads(raw)
        return value if isinstance(value, dict) else {}
    except Exception:
        return {}


pick = load("TT_PICK_MESSAGE")
gate = load("TT_REPLY_GATE")
has_message = bool(pick.get("has_message"))
should_reply = bool(has_message and gate.get("should_reply") is True)
gate_rejected = bool(has_message and gate and gate.get("should_reply") is False)
print(json.dumps({
    "has_message": has_message,
    "should_reply": should_reply,
    "gate_rejected": gate_rejected,
}, ensure_ascii=False))
