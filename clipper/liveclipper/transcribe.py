from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any


class TranscriptionError(Exception):
    pass


@dataclass(frozen=True)
class CaptionResult:
    path: Path | None
    words: int
    warning: str = ""
    entries: list[tuple[float, float, str]] | None = None
    segments: list[dict[str, Any]] | None = None


def ass_time(seconds: float) -> str:
    if seconds < 0:
        seconds = 0
    total_cs = int(round(seconds * 100))
    cs = total_cs % 100
    total_s = total_cs // 100
    s = total_s % 60
    total_m = total_s // 60
    m = total_m % 60
    h = total_m // 60
    return f"{h}:{m:02d}:{s:02d}.{cs:02d}"


def ass_escape_text(text: str) -> str:
    return text.replace("\\", "\\\\").replace("{", "\\{").replace("}", "\\}").replace("\n", " ")


KARAOKE_PRESETS = {"karaoke_pop", "tiktok_pop"}


STYLE_BASE = {
    "default": ("Default", "Arial", 72, 1, 5, 1),
    "tiktok_pop": ("Default", "Arial Black", 80, 1, 6, 0),
    "karaoke_pop": ("Default", "Arial Black", 80, 1, 6, 0),
    "subtitle_bar": ("Default", "Arial", 64, 3, 0, 0),
    "gaming": ("Default", "Impact", 80, 1, 8, 2),
}

CAPTION_SIZE_SCALE = {"sm": 0.78, "md": 1.0, "lg": 1.28}
CAPTION_POSITION_ALIGN = {"bottom": 2, "center": 5, "top": 8}
CAPTION_POSITION_MARGIN = {"bottom": 210, "center": 0, "top": 80}

PLAY_RES_X = 1080
PLAY_RES_Y = 1920


def _ass_pos_from_transform(transform: dict[str, Any]) -> tuple[int, int]:
    x = int(round(float(transform.get("x", 0.5)) * PLAY_RES_X))
    y = int(round(float(transform.get("y", 0.5)) * PLAY_RES_Y))
    return x, y


def _transform_ass_tags(transform: dict[str, Any]) -> str:
    x, y = _ass_pos_from_transform(transform)
    rotation = float(transform.get("rotation", 0))
    scale = float(transform.get("scale", 1.0))
    tags = [f"\\pos({x},{y})", f"\\an5", f"\\frz{rotation:.1f}"]
    if scale != 1.0:
        scale_pct = max(1, int(round(scale * 100)))
        tags.append(f"\\fscx{scale_pct}\\fscy{scale_pct}")
    return "".join(tags)


def _effect_ass_tags(effect: str, x: int, y: int) -> str:
    if effect in ("", "none", None):
        return ""
    if effect == "pop":
        return "\\fscx80\\fscy80\\t(0,150,\\fscx100\\fscy100)"
    if effect == "glow":
        return "\\blur3\\bord4\\3c&HFFFFFF&"
    if effect == "bounce":
        return (
            f"\\t(0,150,\\pos({x},{y - 40}))"
            f"\\t(150,300,\\pos({x},{y}))"
        )
    if effect == "shake":
        return (
            f"\\t(0,50,\\pos({x + 8},{y}))"
            f"\\t(50,100,\\pos({x - 8},{y}))"
            f"\\t(100,150,\\pos({x + 6},{y}))"
            f"\\t(150,200,\\pos({x},{y}))"
        )
    return ""


def _wrap_dialogue_text(
    text: str,
    *,
    transform: dict[str, Any] | None,
    effect: str | None,
) -> str:
    if not transform:
        return text
    x, y = _ass_pos_from_transform(transform)
    tags = [_transform_ass_tags(transform)]
    effect_tag = _effect_ass_tags(str(effect or ""), x, y)
    if effect_tag:
        tags.append(effect_tag)
    return "{" + "".join(tags) + "}" + text


def build_style_line(
    style_preset: str,
    *,
    caption_size: str = "md",
    caption_position: str = "bottom",
) -> str:
    name, font, base_size, border_style, outline, shadow = STYLE_BASE.get(
        style_preset, STYLE_BASE["default"]
    )
    scale = CAPTION_SIZE_SCALE.get(caption_size, 1.0)
    font_size = max(32, int(round(base_size * scale)))
    alignment = CAPTION_POSITION_ALIGN.get(caption_position, 2)
    margin_v = CAPTION_POSITION_MARGIN.get(caption_position, 210)
    colours = {
        "default": "&H00FFFFFF,&H0000FFFF,&H00000000,&H80000000",
        "tiktok_pop": "&H0000FFFF,&H0000FFFF,&H00000000,&H00000000",
        "karaoke_pop": "&H0000FFFF,&H00FFFFFF,&H00000000,&H00000000",
        "subtitle_bar": "&H00FFFFFF,&H0000FFFF,&H00000000,&H80000000",
        "gaming": "&H00FFFFFF,&H0000FFFF,&H00000000,&H00000000",
    }
    colour = colours.get(style_preset, colours["default"])
    return (
        f"Style: {name},{font},{font_size},{colour},-1,0,0,0,100,100,0,0,"
        f"{border_style},{outline},{shadow},{alignment},80,80,{margin_v},1"
    )


def detect_emote_tokens(text: str, emote_names: set[str]) -> list[str]:
    found: list[str] = []
    for part in text.split():
        key = part.strip().lower()
        if key and key in emote_names:
            found.append(part.strip())
    return found


def offset_caption_segments(
    segments: list[dict[str, Any]],
    trim_start: float,
    trim_end: float | None = None,
) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    for seg in segments:
        start = float(seg.get("start", 0)) - trim_start
        end = float(seg.get("end", 0)) - trim_start
        if trim_end is not None:
            if start >= trim_end - trim_start:
                continue
            end = min(end, trim_end - trim_start)
        if end <= 0:
            continue
        start = max(0.0, start)
        text = str(seg.get("text", ""))
        words_raw = seg.get("words")
        words_out: list[dict[str, float | str]] | None = None
        if isinstance(words_raw, list):
            words_out = []
            for w in words_raw:
                ws = float(w.get("start", 0)) - trim_start
                we = float(w.get("end", 0)) - trim_start
                if trim_end is not None:
                    we = min(we, trim_end - trim_start)
                if we <= 0:
                    continue
                ws = max(0.0, ws)
                wt = str(w.get("text", "")).strip()
                if wt:
                    words_out.append({"text": wt, "start": ws, "end": we})
            if not words_out:
                continue
            start = float(words_out[0]["start"])
            end = float(words_out[-1]["end"])
        if not text.strip() and not words_out:
            continue
        entry: dict[str, Any] = {"start": start, "end": end, "text": text}
        if words_out:
            entry["words"] = words_out
            if not text.strip():
                entry["text"] = " ".join(str(w["text"]) for w in words_out)
        result.append(entry)
    return result


def segments_to_flat_entries(segments: list[dict[str, Any]]) -> list[tuple[float, float, str]]:
    return [
        (float(s["start"]), float(s["end"]), str(s.get("text", "")))
        for s in segments
        if str(s.get("text", "")).strip()
    ]


def format_karaoke_text(words: list[dict[str, Any]]) -> str:
    parts: list[str] = []
    for word in words:
        text = ass_escape_text(str(word.get("text", "")).strip())
        if not text:
            continue
        ws = float(word.get("start", 0))
        we = float(word.get("end", ws + 0.2))
        duration_cs = max(1, int(round((we - ws) * 100)))
        parts.append(f"{{\\k{duration_cs}}}{text}")
    return " ".join(parts)


def write_ass(
    path: Path,
    segments: list[dict[str, Any]] | list[tuple[float, float, str]],
    style_preset: str = "default",
    max_words_per_line: int | None = None,
    caption_size: str = "md",
    caption_position: str = "bottom",
    emote_names: set[str] | None = None,
) -> list[dict[str, Any]]:
    path.parent.mkdir(parents=True, exist_ok=True)
    style_str = build_style_line(
        style_preset,
        caption_size=caption_size,
        caption_position=caption_position,
    )
    emote_hits: list[dict[str, Any]] = []
    use_karaoke = style_preset in KARAOKE_PRESETS

    lines = [
        "[Script Info]",
        "ScriptType: v4.00+",
        "PlayResX: 1080",
        "PlayResY: 1920",
        "ScaledBorderAndShadow: yes",
        "",
        "[V4+ Styles]",
        "Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding",
        style_str,
        "",
        "[Events]",
        "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text",
    ]

    normalized: list[dict[str, Any]] = []
    for item in segments:
        if isinstance(item, tuple):
            start, end, text = item
            normalized.append({"start": start, "end": end, "text": text})
        else:
            normalized.append(item)

    for seg in normalized:
        start = float(seg.get("start", 0))
        end = float(seg.get("end", 0))
        text = str(seg.get("text", "")).strip()
        words_raw = seg.get("words")
        transform_raw = seg.get("transform")
        transform = transform_raw if isinstance(transform_raw, dict) else None
        effect = str(seg.get("effect") or "")
        if end <= start:
            end = start + 0.6

        def apply_overrides(dialogue_text: str) -> str:
            return _wrap_dialogue_text(
                dialogue_text,
                transform=transform,
                effect=effect,
            )

        dialogue_text = text
        if use_karaoke and isinstance(words_raw, list) and words_raw:
            if max_words_per_line and max_words_per_line > 0:
                word_chunks: list[list[dict[str, Any]]] = []
                chunk: list[dict[str, Any]] = []
                for w in words_raw:
                    chunk.append(w)
                    if len(chunk) >= max_words_per_line:
                        word_chunks.append(chunk)
                        chunk = []
                if chunk:
                    word_chunks.append(chunk)
                for chunk in word_chunks:
                    if not chunk:
                        continue
                    c_start = float(chunk[0].get("start", start))
                    c_end = float(chunk[-1].get("end", end))
                    karaoke = format_karaoke_text(chunk)
                    if karaoke:
                        lines.append(
                            f"Dialogue: 0,{ass_time(c_start)},{ass_time(c_end)},Default,,0,0,0,,"
                            f"{apply_overrides(karaoke)}"
                        )
                continue
            dialogue_text = format_karaoke_text(words_raw) or ass_escape_text(text)
        else:
            dialogue_text = ass_escape_text(text)

        if not dialogue_text:
            continue
        if emote_names:
            for emote_name in detect_emote_tokens(text, emote_names):
                emote_hits.append(
                    {"name": emote_name, "start": start, "end": end}
                )
        lines.append(
            f"Dialogue: 0,{ass_time(start)},{ass_time(end)},Default,,0,0,0,,"
            f"{apply_overrides(dialogue_text)}"
        )

    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return emote_hits


def group_words_into_segments(
    words: list[dict[str, Any]],
    max_words_per_line: int = 3,
) -> list[dict[str, Any]]:
    segments: list[dict[str, Any]] = []
    buffer: list[dict[str, Any]] = []
    for word in words:
        text = str(word.get("text", "")).strip()
        if not text:
            continue
        buffer.append(word)
        if len(buffer) >= max_words_per_line:
            segments.append(_segment_from_words(buffer))
            buffer = []
    if buffer:
        segments.append(_segment_from_words(buffer))
    return segments


def _segment_from_words(words: list[dict[str, Any]]) -> dict[str, Any]:
    return {
        "start": float(words[0].get("start", 0)),
        "end": float(words[-1].get("end", 0)),
        "text": " ".join(str(w.get("text", "")).strip() for w in words),
        "words": list(words),
    }


class Transcriber:
    def __init__(self, model_name: str, compute_type: str, enabled: bool, required: bool):
        self.model_name = model_name
        self.compute_type = compute_type
        self.enabled = enabled
        self.required = required
        self._model = None

    def transcribe(
        self,
        video_path: Path,
        captions_path: Path,
        *,
        trim_start: float = 0.0,
        trim_duration: float | None = None,
        max_words_per_line: int = 3,
    ) -> CaptionResult:
        if not self.enabled:
            return CaptionResult(None, 0, "transcription disabled", entries=[], segments=[])
        try:
            from faster_whisper import WhisperModel
        except ImportError as exc:
            if self.required:
                raise TranscriptionError("faster-whisper is not installed") from exc
            return CaptionResult(None, 0, "faster-whisper is not installed; rendered without captions", entries=[], segments=[])
        if self._model is None:
            self._model = WhisperModel(self.model_name, compute_type=self.compute_type)
        try:
            kwargs: dict[str, Any] = {"word_timestamps": True}
            if trim_start > 0:
                if trim_duration is not None:
                    kwargs["clip_timestamps"] = [trim_start, trim_start + trim_duration]
                else:
                    kwargs["clip_timestamps"] = [trim_start]
            segments_iter, _ = self._model.transcribe(str(video_path), **kwargs)
            all_words: list[dict[str, Any]] = []
            for segment in segments_iter:
                words = getattr(segment, "words", None) or []
                if words:
                    for word in words:
                        text = str(getattr(word, "word", "") or "").strip()
                        if not text:
                            continue
                        ws = float(getattr(word, "start", 0) or 0)
                        we = float(getattr(word, "end", 0) or 0)
                        if trim_duration is not None and we > trim_start + trim_duration:
                            we = trim_start + trim_duration
                        if trim_start > 0 and ws < trim_start:
                            ws = trim_start
                        all_words.append({"text": text, "start": ws, "end": we})
                else:
                    text = str(getattr(segment, "text", "") or "").strip()
                    if text:
                        ws = float(getattr(segment, "start", 0) or 0)
                        we = float(getattr(segment, "end", 0) or 0)
                        all_words.append({"text": text, "start": ws, "end": we})

            if not all_words:
                return CaptionResult(None, 0, "transcript was empty; rendered without captions", entries=[], segments=[])

            caption_segments = group_words_into_segments(all_words, max_words_per_line=max_words_per_line)
            flat = segments_to_flat_entries(caption_segments)
            write_ass(captions_path, caption_segments)
            return CaptionResult(
                captions_path,
                len(caption_segments),
                entries=flat,
                segments=caption_segments,
            )
        except Exception as exc:
            if self.required:
                raise TranscriptionError(str(exc)) from exc
            return CaptionResult(None, 0, f"transcription failed: {exc}; rendered without captions", entries=[], segments=[])
