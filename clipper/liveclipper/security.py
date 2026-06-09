from __future__ import annotations

def check_webhook_token(request: object, expected: str) -> None:
    from fastapi import HTTPException

    if not expected:
        return
    headers = getattr(request, "headers")
    auth = headers.get("authorization", "")
    token = headers.get("x-clipper-token", "")
    if auth.lower().startswith("bearer "):
        token = auth[7:].strip()
    if token != expected:
        raise HTTPException(status_code=401, detail={"error": "unauthorized"})


def redact(value: str) -> str:
    if not value:
        return ""
    if len(value) <= 8:
        return "<redacted>"
    return value[:4] + "<redacted>" + value[-4:]
