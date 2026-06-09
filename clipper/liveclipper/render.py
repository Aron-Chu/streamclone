from __future__ import annotations

import subprocess
from dataclasses import dataclass
from pathlib import Path

from .templates import AudioEffects, IntroZoom, VideoEffects


class RenderError(Exception):
    pass


@dataclass(frozen=True)
class RenderPlan:
    trim_start: float
    trim_duration: float
    command: list[str]


def compute_trim_start(
    *,
    source_duration: float,
    final_duration: float,
    event_latency_offset: float,
    trigger_detected_at_ms: int,
    peak_chat_ts_ms: int | None,
) -> float:
    if source_duration <= final_duration:
        return 0.0
    if peak_chat_ts_ms:
        delta = (peak_chat_ts_ms - trigger_detected_at_ms) / 1000
        event_time = source_duration + delta - event_latency_offset
        start = event_time - final_duration * 0.35
    else:
        start = source_duration - final_duration
    return max(0.0, min(start, source_duration - final_duration))


def escape_filter_path(path: Path) -> str:
    text = str(path).replace("\\", "/")
    escaped = []
    for ch in text:
        if ch in {"'", ":", ",", "[", "]"}:
            escaped.append("\\" + ch)
        else:
            escaped.append(ch)
    return "".join(escaped)


def _output_size(format_preset: str) -> tuple[int, int]:
    if format_preset == "youtube":
        return 1920, 1080
    if format_preset == "twitter":
        return 1080, 1080
    return 1080, 1920


def _layout_filter(format_preset: str) -> str:
    if format_preset in {"tiktok", "youtube_short", "instagram_reel"}:
        return (
            "[0:v]split=2[bg][fg];"
            "[bg]scale=1080:1920:force_original_aspect_ratio=increase,"
            "crop=1080:1920,boxblur=20:5[base];"
            "[fg]scale=1080:1920:force_original_aspect_ratio=decrease,"
            "pad=1080:1920:(ow-iw)/2:(oh-ih)/2[vfg];"
            "[base][vfg]overlay=0:0[vpre];"
        )
    if format_preset == "youtube":
        return (
            "[0:v]scale=1920:1080:force_original_aspect_ratio=decrease,"
            "pad=1920:1080:(ow-iw)/2:(oh-ih)/2[vpre];"
        )
    if format_preset == "twitter":
        return (
            "[0:v]split=2[bg][fg];"
            "[bg]scale=1080:1080:force_original_aspect_ratio=increase,"
            "crop=1080:1080,boxblur=20:5[base];"
            "[fg]scale=1080:1080:force_original_aspect_ratio=decrease,"
            "pad=1080:1080:(ow-iw)/2:(oh-ih)/2[vfg];"
            "[base][vfg]overlay=0:0[vpre];"
        )
    return (
        "[0:v]split=2[bg][fg];"
        "[bg]scale=1080:1920:force_original_aspect_ratio=increase,"
        "crop=1080:1920,boxblur=20:5[base];"
        "[fg]scale=1080:1920:force_original_aspect_ratio=decrease,"
        "pad=1080:1920:(ow-iw)/2:(oh-ih)/2[vfg];"
        "[base][vfg]overlay=0:0[vpre];"
    )


def _intro_zoom_filter(intro_zoom: IntroZoom, width: int, height: int, fps: int = 30) -> str:
    frames = max(1, int(intro_zoom.duration * fps))
    scale = intro_zoom.scale
    z_expr = f"if(lte(on\\,{frames})\\,1+({scale}-1)*(1-on/{frames})\\,1)"
    return (
        f"zoompan=z='{z_expr}':d=1:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':"
        f"s={width}x{height}:fps={fps}"
    )


def _apply_video_effects(
    input_label: str,
    output_label: str,
    effects: VideoEffects | None,
    format_preset: str,
) -> str:
    width, height = _output_size(format_preset)
    chain = f"[{input_label}]"
    current = input_label
    parts: list[str] = []

    if effects and effects.intro_zoom:
        zoom_label = "vzoom"
        parts.append(f"{chain}{_intro_zoom_filter(effects.intro_zoom, width, height)}[{zoom_label}]")
        chain = f"[{zoom_label}]"
        current = zoom_label

    if effects and effects.vignette:
        vignette_label = "vvig"
        parts.append(f"{chain}vignette=PI/4[{vignette_label}]")
        chain = f"[{vignette_label}]"
        current = vignette_label

    if current == input_label:
        parts.append(f"[{input_label}]null[{output_label}]")
    else:
        parts.append(f"{chain}null[{output_label}]")

    return "".join(parts)


def build_filter(
    captions_path: Path | None,
    format_preset: str = "tiktok",
    video_effects: VideoEffects | None = None,
) -> str:
    base = _layout_filter(format_preset)
    if captions_path:
        post = _apply_video_effects("vpre", "vpost", video_effects, format_preset)
        return base + post + f";[vpost]subtitles='{escape_filter_path(captions_path)}'[v]"
    post = _apply_video_effects("vpre", "v", video_effects, format_preset)
    return base + post


def build_audio_filter(effects: AudioEffects | None) -> str | None:
    if not effects:
        return None
    parts: list[str] = []
    if effects.noise_reduce == "light":
        parts.append("afftdn=nr=12:nf=-25")
    if effects.normalize:
        parts.append("loudnorm")
    if not parts:
        return None
    return ",".join(parts)


def build_command(
    *,
    ffmpeg_bin: str,
    input_path: Path,
    output_path: Path,
    captions_path: Path | None,
    source_duration: float,
    final_duration: float,
    event_latency_offset: float,
    trigger_detected_at_ms: int,
    peak_chat_ts_ms: int | None,
    encoder: str,
    preset: str,
    format_preset: str = "tiktok",
    trim_start: float | None = None,
    video_effects: VideoEffects | None = None,
    audio_effects: AudioEffects | None = None,
) -> RenderPlan:
    if trim_start is None:
        trim_start = compute_trim_start(
            source_duration=source_duration,
            final_duration=final_duration,
            event_latency_offset=event_latency_offset,
            trigger_detected_at_ms=trigger_detected_at_ms,
            peak_chat_ts_ms=peak_chat_ts_ms,
        )
    audio_filter = build_audio_filter(audio_effects)
    command = [
        ffmpeg_bin,
        "-y",
        "-hide_banner",
        "-loglevel",
        "error",
        "-ss",
        f"{trim_start:.3f}",
        "-t",
        f"{final_duration:.3f}",
        "-i",
        str(input_path),
        "-filter_complex",
        build_filter(captions_path, format_preset, video_effects),
        "-map",
        "[v]",
    ]
    if audio_filter:
        command.extend(["-af", audio_filter])
    command.extend(
        [
            "-map",
            "0:a?",
            "-c:v",
            encoder,
            "-preset",
            preset,
            "-c:a",
            "aac",
            "-b:a",
            "160k",
            "-movflags",
            "+faststart",
            str(output_path),
        ]
    )
    return RenderPlan(trim_start=trim_start, trim_duration=final_duration, command=command)


class Renderer:
    def __init__(self, encoder: str, preset: str, ffmpeg_bin: str = "ffmpeg"):
        self.encoder = encoder
        self.preset = preset
        self.ffmpeg_bin = self._resolve_ffmpeg(ffmpeg_bin)

    def render(
        self,
        *,
        input_path: Path,
        output_path: Path,
        captions_path: Path | None,
        source_duration: float,
        final_duration: float,
        event_latency_offset: float,
        trigger_detected_at_ms: int,
        peak_chat_ts_ms: int | None,
        format_preset: str = "tiktok",
        trim_start: float | None = None,
        video_effects: VideoEffects | None = None,
        audio_effects: AudioEffects | None = None,
    ) -> RenderPlan:
        output_path.parent.mkdir(parents=True, exist_ok=True)
        plan = build_command(
            ffmpeg_bin=self.ffmpeg_bin,
            input_path=input_path,
            output_path=output_path,
            captions_path=captions_path,
            source_duration=source_duration,
            final_duration=final_duration,
            event_latency_offset=event_latency_offset,
            trigger_detected_at_ms=trigger_detected_at_ms,
            peak_chat_ts_ms=peak_chat_ts_ms,
            encoder=self.encoder,
            preset=self.preset,
            format_preset=format_preset,
            trim_start=trim_start,
            video_effects=video_effects,
            audio_effects=audio_effects,
        )
        try:
            proc = subprocess.run(
                plan.command,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=600,
                check=False,
            )
        except FileNotFoundError as exc:
            raise RenderError("ffmpeg was not found on PATH") from exc
        except subprocess.TimeoutExpired as exc:
            raise RenderError("ffmpeg render timed out") from exc
        if proc.returncode != 0:
            raise RenderError((proc.stderr or "").strip() or "ffmpeg failed")
        return plan

    def _resolve_ffmpeg(self, ffmpeg_bin: str) -> str:
        if ffmpeg_bin != "ffmpeg":
            return ffmpeg_bin
        try:
            import imageio_ffmpeg

            return imageio_ffmpeg.get_ffmpeg_exe()
        except Exception:
            return ffmpeg_bin
