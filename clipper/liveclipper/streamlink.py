from __future__ import annotations

import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from shutil import which

from .security import redact


class DownloadError(Exception):
    def __init__(self, code: str, message: str):
        super().__init__(message)
        self.code = code
        self.message = message


@dataclass(frozen=True)
class CommandPreview:
    argv: list[str]
    redacted: list[str]


def build_command(
    clip_url: str,
    output_path: Path,
    token: str = "",
    streamlink_bin: str = "streamlink",
) -> CommandPreview:
    argv = [streamlink_bin, clip_url, "best", "--force", "--output", str(output_path)]
    if token:
        header = "Authorization=Bearer " + token
        argv.insert(1, "--twitch-api-header=" + header)
    redacted = []
    for arg in argv:
        if token and token in arg:
            redacted.append(arg.replace(token, redact(token)))
        else:
            redacted.append(arg)
    return CommandPreview(argv=argv, redacted=redacted)


class StreamlinkDownloader:
    def __init__(self, token: str = "", streamlink_bin: str = "streamlink"):
        self.token = token
        self.streamlink_bin = resolve_streamlink_bin(streamlink_bin)

    def download(self, clip_url: str, output_path: Path) -> list[str]:
        output_path.parent.mkdir(parents=True, exist_ok=True)
        if self.token:
            preview = build_command(clip_url, output_path, self.token, self.streamlink_bin)
            try:
                self._run(preview.argv)
                return preview.redacted
            except DownloadError as exc:
                if exc.code not in {"download_failed", "download_auth_failed"}:
                    raise
                anonymous = build_command(clip_url, output_path, streamlink_bin=self.streamlink_bin)
                self._run(anonymous.argv)
                return anonymous.redacted
        preview = build_command(clip_url, output_path, streamlink_bin=self.streamlink_bin)
        self._run(preview.argv)
        return preview.redacted

    def _run(self, argv: list[str]) -> None:
        try:
            proc = subprocess.run(
                argv,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=180,
                check=False,
            )
        except FileNotFoundError as exc:
            raise DownloadError("streamlink_missing", f"{argv[0]} was not found") from exc
        except subprocess.TimeoutExpired as exc:
            raise DownloadError("download_timeout", "streamlink download timed out") from exc
        if proc.returncode != 0:
            stderr = (proc.stderr or "").strip()
            lower = stderr.lower()
            code = "download_auth_failed" if "403" in lower or "forbidden" in lower or "unauthorized" in lower else "download_failed"
            raise DownloadError(code, stderr or "streamlink failed")


def resolve_streamlink_bin(streamlink_bin: str) -> str:
    if streamlink_bin != "streamlink":
        return streamlink_bin
    found = which(streamlink_bin)
    if found:
        return found
    sibling = Path(sys.executable).with_name("streamlink")
    if sibling.exists():
        return str(sibling)
    return streamlink_bin
