from __future__ import annotations

import json
import subprocess
from dataclasses import dataclass
from pathlib import Path
from shutil import which

from .security import redact
from .streamlink import DownloadError, resolve_streamlink_bin


class VodDownloadError(DownloadError):
    pass


@dataclass(frozen=True)
class VodSegmentWindow:
    segment_start: float
    duration: float
    moment_offset: float


@dataclass(frozen=True)
class CommandPreview:
    argv: list[str]
    redacted: list[str]


def build_vod_page_url(vod_id: str) -> str:
    vod_id = str(vod_id).strip()
    if not vod_id:
        raise VodDownloadError("vod_invalid", "vod_id is required")
    return f"https://www.twitch.tv/videos/{vod_id}"


def compute_vod_segment_window(vod_offset_seconds: float, source_duration: float) -> VodSegmentWindow:
    duration = max(5.0, float(source_duration))
    offset = max(0.0, float(vod_offset_seconds))
    segment_start = max(0.0, offset - duration / 2)
    moment_offset = offset - segment_start
    return VodSegmentWindow(
        segment_start=segment_start,
        duration=duration,
        moment_offset=moment_offset,
    )


def build_streamlink_json_argv(
    vod_url: str,
    *,
    token: str = "",
    streamlink_bin: str = "streamlink",
) -> CommandPreview:
    argv = [streamlink_bin, "--json", vod_url, "best"]
    if token:
        header = "Authorization=Bearer " + token
        argv.insert(1, "--twitch-api-header=" + header)
    return _redact_argv(argv, token)


def build_ffmpeg_segment_argv(
    *,
    ffmpeg_bin: str,
    stream_url: str,
    output_path: Path,
    segment_start: float,
    duration: float,
) -> list[str]:
    return [
        ffmpeg_bin,
        "-y",
        "-hide_banner",
        "-loglevel",
        "error",
        "-ss",
        f"{segment_start:.3f}",
        "-t",
        f"{duration:.3f}",
        "-i",
        stream_url,
        "-c",
        "copy",
        "-bsf:a",
        "aac_adtstoasc",
        "-movflags",
        "+faststart",
        str(output_path),
    ]


def parse_streamlink_json(stdout: str) -> str:
    try:
        payload = json.loads(stdout)
    except json.JSONDecodeError as exc:
        raise VodDownloadError("vod_resolve_failed", "streamlink returned invalid JSON") from exc
    if isinstance(payload, dict):
        url = payload.get("url") or payload.get("master") or payload.get("stream")
        if isinstance(url, str) and url.strip():
            return url.strip()
        streams = payload.get("streams")
        if isinstance(streams, dict):
            for entry in streams.values():
                if isinstance(entry, dict):
                    nested = entry.get("url")
                    if isinstance(nested, str) and nested.strip():
                        return nested.strip()
    raise VodDownloadError("vod_resolve_failed", "streamlink JSON did not include a stream URL")


def _redact_argv(argv: list[str], token: str) -> CommandPreview:
    redacted: list[str] = []
    for arg in argv:
        if token and token in arg:
            redacted.append(arg.replace(token, redact(token)))
        else:
            redacted.append(arg)
    return CommandPreview(argv=argv, redacted=redacted)


def resolve_ffmpeg_bin(ffmpeg_bin: str) -> str:
    if ffmpeg_bin != "ffmpeg":
        return ffmpeg_bin
    try:
        import imageio_ffmpeg

        return imageio_ffmpeg.get_ffmpeg_exe()
    except Exception:
        found = which("ffmpeg")
        return found or ffmpeg_bin


class VodDownloader:
    def __init__(
        self,
        *,
        token: str = "",
        streamlink_bin: str = "streamlink",
        ffmpeg_bin: str = "ffmpeg",
    ):
        self.token = token
        self.streamlink_bin = resolve_streamlink_bin(streamlink_bin)
        self.ffmpeg_bin = resolve_ffmpeg_bin(ffmpeg_bin)

    def resolve_stream_url(self, vod_id: str) -> str:
        vod_url = build_vod_page_url(vod_id)
        preview = build_streamlink_json_argv(
            vod_url,
            token=self.token,
            streamlink_bin=self.streamlink_bin,
        )
        stdout = self._run_json(preview.argv)
        return parse_streamlink_json(stdout)

    def download_segment(
        self,
        *,
        vod_id: str,
        output_path: Path,
        offset_seconds: float,
        duration: float,
    ) -> VodSegmentWindow:
        window = compute_vod_segment_window(offset_seconds, duration)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        stream_url = self.resolve_stream_url(vod_id)
        argv = build_ffmpeg_segment_argv(
            ffmpeg_bin=self.ffmpeg_bin,
            stream_url=stream_url,
            output_path=output_path,
            segment_start=window.segment_start,
            duration=window.duration,
        )
        self._run_ffmpeg(argv)
        return window

    def _run_json(self, argv: list[str]) -> str:
        try:
            proc = subprocess.run(
                argv,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=120,
                check=False,
            )
        except FileNotFoundError as exc:
            raise VodDownloadError("streamlink_missing", f"{argv[0]} was not found") from exc
        except subprocess.TimeoutExpired as exc:
            raise VodDownloadError("vod_resolve_timeout", "streamlink VOD resolve timed out") from exc
        if proc.returncode != 0:
            stderr = (proc.stderr or "").strip()
            lower = stderr.lower()
            code = (
                "vod_auth_failed"
                if "403" in lower or "forbidden" in lower or "unauthorized" in lower
                else "vod_resolve_failed"
            )
            raise VodDownloadError(code, stderr or "streamlink VOD resolve failed")
        return proc.stdout or ""

    def _run_ffmpeg(self, argv: list[str]) -> None:
        try:
            proc = subprocess.run(
                argv,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=900,
                check=False,
            )
        except FileNotFoundError as exc:
            raise VodDownloadError("ffmpeg_missing", f"{argv[0]} was not found") from exc
        except subprocess.TimeoutExpired as exc:
            raise VodDownloadError("vod_download_timeout", "ffmpeg VOD segment download timed out") from exc
        if proc.returncode != 0:
            stderr = (proc.stderr or "").strip()
            lower = stderr.lower()
            code = (
                "vod_auth_failed"
                if "403" in lower or "forbidden" in lower or "unauthorized" in lower
                else "vod_download_failed"
            )
            raise VodDownloadError(code, stderr or "ffmpeg VOD segment download failed")
