from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path


def _env_file_candidates() -> tuple[Path, ...]:
    repo_root = Path(__file__).resolve().parents[2]
    return (
        Path(".env"),
        Path("..") / ".env",
        repo_root / ".env",
    )


def _load_env_files() -> None:
    for path in _env_file_candidates():
        if not path.exists():
            continue
        for raw in path.read_text(encoding="utf-8").splitlines():
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            key = key.strip()
            value = value.strip().strip('"').strip("'")
            if key and key not in os.environ:
                os.environ[key] = value


def _str(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


def _int(name: str, default: int) -> int:
    raw = _str(name)
    if raw == "":
        return default
    return int(raw)


def _float(name: str, default: float) -> float:
    raw = _str(name)
    if raw == "":
        return default
    return float(raw)


def _bool(name: str, default: bool) -> bool:
    raw = _str(name)
    if raw == "":
        return default
    return raw.lower() in {"1", "true", "yes", "on"}


@dataclass(frozen=True)
class Config:
    host: str
    port: int
    webhook_token: str
    twitch_client_id: str
    twitch_user_access_token: str
    twitch_api_url: str
    twitch_irc_url: str
    data_dir: Path
    db_path: Path
    output_dir: Path
    source_duration: float
    final_duration: float
    event_latency_offset: float
    duplicate_window_seconds: int
    cooldown_seconds: int
    clip_poll_timeout_seconds: int
    clip_poll_interval_seconds: float
    chat_window_seconds: int
    chat_min_messages: int
    chat_spike_multiplier: float
    asr_enabled: bool
    asr_required: bool
    whisper_model: str
    whisper_compute_type: str
    streamlink_bin: str
    ffmpeg_encoder: str
    ffmpeg_preset: str
    ffmpeg_bin: str
    final_retention_hours: int
    cleanup_interval_seconds: int
    stale_job_seconds: int
    auto_render: bool


def load_config() -> Config:
    _load_env_files()
    data_dir = Path(_str("CLIPPER_DATA_DIR", "clipper-data"))
    output_dir = Path(_str("CLIPPER_OUTPUT_DIR", str(data_dir / "output")))
    db_path = Path(_str("CLIPPER_DB_PATH", str(data_dir / "clipper.sqlite")))
    return Config(
        host=_str("CLIPPER_HOST", "127.0.0.1"),
        port=_int("CLIPPER_PORT", 8095),
        webhook_token=_str("CLIPPER_WEBHOOK_TOKEN"),
        twitch_client_id=_str("CLIPPER_TWITCH_CLIENT_ID") or _str("TWITCH_OAUTH_CLIENT_ID"),
        twitch_user_access_token=_str("CLIPPER_TWITCH_USER_ACCESS_TOKEN"),
        twitch_api_url=_str("CLIPPER_TWITCH_API_URL", "https://api.twitch.tv/helix").rstrip("/"),
        twitch_irc_url=_str("CLIPPER_TWITCH_IRC_URL", "wss://irc-ws.chat.twitch.tv:443"),
        data_dir=data_dir,
        db_path=db_path,
        output_dir=output_dir,
        source_duration=_float("CLIPPER_SOURCE_DURATION", 60.0),
        final_duration=_float("CLIPPER_FINAL_DURATION", 30.0),
        event_latency_offset=_float("CLIPPER_EVENT_LATENCY_OFFSET_SECONDS", 8.0),
        duplicate_window_seconds=_int("CLIPPER_DUPLICATE_WINDOW_SECONDS", 60),
        cooldown_seconds=_int("CLIPPER_COOLDOWN_SECONDS", 120),
        clip_poll_timeout_seconds=_int("CLIPPER_CLIP_POLL_TIMEOUT_SECONDS", 60),
        clip_poll_interval_seconds=_float("CLIPPER_CLIP_POLL_INTERVAL_SECONDS", 1.0),
        chat_window_seconds=_int("CLIPPER_CHAT_WINDOW_SECONDS", 12),
        chat_min_messages=_int("CLIPPER_CHAT_MIN_MESSAGES", 18),
        chat_spike_multiplier=_float("CLIPPER_CHAT_SPIKE_MULTIPLIER", 2.5),
        asr_enabled=_bool("CLIPPER_ASR_ENABLED", True),
        asr_required=_bool("CLIPPER_ASR_REQUIRED", False),
        whisper_model=_str("CLIPPER_WHISPER_MODEL", "small"),
        whisper_compute_type=_str("CLIPPER_WHISPER_COMPUTE_TYPE", "int8"),
        streamlink_bin=_str("CLIPPER_STREAMLINK_BIN", "streamlink"),
        ffmpeg_encoder=_str("CLIPPER_FFMPEG_ENCODER", "libx264"),
        ffmpeg_preset=_str("CLIPPER_FFMPEG_PRESET", "veryfast"),
        ffmpeg_bin=_str("CLIPPER_FFMPEG_BIN", "ffmpeg"),
        final_retention_hours=_int("CLIPPER_FINAL_RETENTION_HOURS", 48),
        cleanup_interval_seconds=_int("CLIPPER_CLEANUP_INTERVAL_SECONDS", 3600),
        stale_job_seconds=_int("CLIPPER_STALE_JOB_SECONDS", 120),
        auto_render=_bool("CLIPPER_AUTO_RENDER", False),
    )


def ensure_dirs(cfg: Config) -> None:
    cfg.data_dir.mkdir(parents=True, exist_ok=True)
    cfg.output_dir.mkdir(parents=True, exist_ok=True)
    cfg.db_path.parent.mkdir(parents=True, exist_ok=True)
