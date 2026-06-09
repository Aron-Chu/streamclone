from __future__ import annotations

import json
import time
from dataclasses import dataclass
from typing import Any
from urllib import error, parse, request


class TwitchError(Exception):
    def __init__(self, code: str, message: str):
        super().__init__(message)
        self.code = code
        self.message = message


@dataclass(frozen=True)
class CreatedClip:
    clip_id: str
    edit_url: str


@dataclass(frozen=True)
class ReadyClip:
    clip_id: str
    url: str
    duration: float
    title: str


class TwitchClient:
    def __init__(self, api_url: str, client_id: str, access_token: str):
        self.api_url = api_url.rstrip("/")
        self.client_id = client_id
        self.access_token = access_token

    def enabled(self) -> bool:
        return bool(self.api_url and self.client_id and self.access_token)

    def resolve_broadcaster_id(self, login: str) -> str:
        data = self._request("GET", "/users", {"login": login})
        items = data.get("data") or []
        if not items:
            raise TwitchError("channel_not_found", "Twitch user was not found")
        broadcaster_id = str(items[0].get("id") or "")
        if not broadcaster_id:
            raise TwitchError("channel_not_found", "Twitch user response did not include an id")
        return broadcaster_id

    def create_clip(
        self,
        broadcaster_id: str,
        title: str,
        duration: float | None,
    ) -> CreatedClip:
        params: dict[str, Any] = {"broadcaster_id": broadcaster_id}
        if title:
            params["title"] = title
        if duration:
            params["duration"] = f"{duration:.1f}"
        data = self._request("POST", "/clips", params)
        items = data.get("data") or []
        if not items:
            raise TwitchError("clip_create_failed", "Twitch returned no clip data")
        clip_id = str(items[0].get("id") or "")
        if not clip_id:
            raise TwitchError("clip_create_failed", "Twitch returned no clip id")
        return CreatedClip(clip_id=clip_id, edit_url=str(items[0].get("edit_url") or ""))

    def get_clip(self, clip_id: str) -> ReadyClip | None:
        data = self._request("GET", "/clips", {"id": clip_id})
        items = data.get("data") or []
        if not items:
            return None
        item = items[0]
        url = str(item.get("url") or "")
        if not url:
            return None
        duration = float(item.get("duration") or 0)
        return ReadyClip(
            clip_id=clip_id,
            url=url,
            duration=duration,
            title=str(item.get("title") or ""),
        )

    def poll_clip(self, clip_id: str, timeout_seconds: int, interval_seconds: float) -> ReadyClip:
        deadline = time.monotonic() + timeout_seconds
        while time.monotonic() < deadline:
            clip = self.get_clip(clip_id)
            if clip:
                return clip
            time.sleep(interval_seconds)
        raise TwitchError("clip_not_ready", "Twitch clip did not become ready before timeout")

    def _request(self, method: str, path: str, params: dict[str, Any]) -> dict[str, Any]:
        if not self.enabled():
            raise TwitchError("twitch_not_configured", "Twitch client id and user access token are required")
        url = self.api_url + path
        encoded = parse.urlencode(params, doseq=True)
        if encoded:
            url += "?" + encoded
        req = request.Request(url, method=method)
        req.add_header("Client-Id", self.client_id)
        req.add_header("Authorization", "Bearer " + self.access_token)
        req.add_header("Accept", "application/json")
        try:
            with request.urlopen(req, timeout=15) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except error.HTTPError as exc:
            message = exc.read().decode("utf-8", errors="replace")
            raise self._http_error(exc.code, message) from exc
        except error.URLError as exc:
            raise TwitchError("twitch_network_error", str(exc)) from exc
        except json.JSONDecodeError as exc:
            raise TwitchError("twitch_bad_response", str(exc)) from exc

    def _http_error(self, status: int, message: str) -> TwitchError:
        lower = message.lower()
        if status == 401:
            if "scope" in lower:
                return TwitchError("missing_scope", message)
            return TwitchError("invalid_token", message)
        if status == 403:
            if "clip" in lower:
                return TwitchError("clip_restricted", message)
            return TwitchError("forbidden", message)
        if status == 404:
            if "live" in lower or "online" in lower or "broadcaster" in lower:
                return TwitchError("offline", message)
            return TwitchError("not_found", message)
        return TwitchError("twitch_http_error", f"Helix returned {status}: {message}")
