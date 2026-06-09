from __future__ import annotations

import re


CHANNEL_RE = re.compile(r"^[a-z0-9][a-z0-9_]{2,24}$")


def normalize_channel(value: str) -> str:
    value = (value or "").strip().lower()
    if value.startswith("#"):
        value = value[1:]
    if not CHANNEL_RE.match(value):
        raise ValueError("invalid channel")
    return value
