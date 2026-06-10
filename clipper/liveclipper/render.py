from __future__ import annotations

import subprocess
from dataclasses import dataclass
from pathlib import Path

from .emote_overlay import (
    build_caption_emote_overlays,
    build_reaction_strip_filter,
    prepare_emote_assets,
    prepare_top_emote_assets,
)
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


def _blur_bg_center_filter(width: int, height: int) -> str:
    return (
        "[0:v]split=2[bg][fg];"
        f"[bg]scale={width}:{height}:force_original_aspect_ratio=increase,"
        f"crop={width}:{height},boxblur=20:5[base];"
        f"[fg]scale={width}:{height}:force_original_aspect_ratio=decrease,"
        f"pad={width}:{height}:(ow-iw)/2:(oh-ih)/2[vfg];"
        "[base][vfg]overlay=0:0[vpre];"
    )


def _stacked_game_face_filter(width: int, height: int, split_ratio: float) -> str:
    ratio = max(0.2, min(0.6, split_ratio))
    top_h = int(height * ratio)
    bottom_h = height - top_h
    return (
        "[0:v]split=2[topsrc][botsrc];"
        f"[topsrc]crop=iw:ih*{ratio:.4f}:0:0,scale={width}:{top_h}:"
        "force_original_aspect_ratio=increase,"
        f"crop={width}:{top_h}[face];"
        f"[botsrc]crop=iw:ih*{1 - ratio:.4f}:0:ih*{ratio:.4f},scale={width}:{bottom_h}:"
        "force_original_aspect_ratio=increase,"
        f"crop={width}:{bottom_h}[game];"
        "[face][game]vstack=inputs=2[vpre];"
    )


def _layout_filter(
    format_preset: str,
    *,
    layout: str = "blur_bg_center",
    layout_split_ratio: float = 0.35,
) -> str:
    width, height = _output_size(format_preset)
    if layout == "stacked_game_face":
        return _stacked_game_face_filter(width, height, layout_split_ratio)
    if format_preset == "youtube":
        return (
            f"[0:v]scale={width}:{height}:force_original_aspect_ratio=decrease,"
            f"pad={width}:{height}:(ow-iw)/2:(oh-ih)/2[vpre];"
        )
    if format_preset == "twitter":
        return (
            "[0:v]split=2[bg][fg];"
            f"[bg]scale={width}:{height}:force_original_aspect_ratio=increase,"
            f"crop={width}:{height},boxblur=20:5[base];"
            f"[fg]scale={width}:{height}:force_original_aspect_ratio=decrease,"
            f"pad={width}:{height}:(ow-iw)/2:(oh-ih)/2[vfg];"
            "[base][vfg]overlay=0:0[vpre];"
        )
    return _blur_bg_center_filter(width, height)


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
    *,
    layout: str | None = None,
    layout_split_ratio: float = 0.35,
    emote_assets_dir: Path | None = None,
    emote_map: dict[str, str] | None = None,
    emote_hits: list[dict] | None = None,
    moment_context: dict | None = None,
) -> str:
    active_layout = layout or (video_effects.layout if video_effects else "blur_bg_center")
    base = _layout_filter(
        format_preset,
        layout=active_layout,
        layout_split_ratio=layout_split_ratio,
    )
    width, height = _output_size(format_preset)
    post = _apply_video_effects("vpre", "vfx", video_effects, format_preset)
    chain = base + post

    current = "vfx"
    assets: dict[str, Path] = {}
    if emote_assets_dir:
        if emote_map:
            assets.update(prepare_emote_assets(emote_map, emote_assets_dir))
        top_emotes_raw = []
        if moment_context and isinstance(moment_context.get("top_emotes"), list):
            top_emotes_raw = moment_context["top_emotes"]
            assets.update(prepare_top_emote_assets(top_emotes_raw, emote_assets_dir))

    top_emotes = []
    if moment_context and isinstance(moment_context.get("top_emotes"), list):
        top_emotes = moment_context["top_emotes"]
    if top_emotes and assets:
        chain += ";" + build_reaction_strip_filter(
            input_label=current,
            output_label="vreact",
            top_emotes=top_emotes,
            assets=assets,
            width=width,
        )
        current = "vreact"

    if captions_path:
        chain += f";[{current}]subtitles='{escape_filter_path(captions_path)}'[vsub]"
        current = "vsub"
    else:
        chain += f";[{current}]null[vsub]"
        current = "vsub"

    if emote_hits and assets:
        chain += ";" + build_caption_emote_overlays(
            input_label=current,
            output_label="v",
            emote_hits=emote_hits,
            assets=assets,
            width=width,
            height=height,
        )
    else:
        chain += f";[{current}]null[v]"
    return chain


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
    layout: str | None = None,
    layout_split_ratio: float = 0.35,
    emote_assets_dir: Path | None = None,
    emote_map: dict[str, str] | None = None,
    emote_hits: list[dict] | None = None,
    moment_context: dict | None = None,
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
        build_filter(
            captions_path,
            format_preset,
            video_effects,
            layout=layout,
            layout_split_ratio=layout_split_ratio,
            emote_assets_dir=emote_assets_dir,
            emote_map=emote_map,
            emote_hits=emote_hits,
            moment_context=moment_context,
        ),
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
        layout: str | None = None,
        layout_split_ratio: float = 0.35,
        emote_assets_dir: Path | None = None,
        emote_map: dict[str, str] | None = None,
        emote_hits: list[dict] | None = None,
        moment_context: dict | None = None,
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
            layout=layout,
            layout_split_ratio=layout_split_ratio,
            emote_assets_dir=emote_assets_dir,
            emote_map=emote_map,
            emote_hits=emote_hits,
            moment_context=moment_context,
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
