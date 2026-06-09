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


STYLE_LINES = {
    "default": "Style: Default,Arial,72,&H00FFFFFF,&H0000FFFF,&H00000000,&H80000000,-1,0,0,0,100,100,0,0,1,5,1,2,80,80,210,1",
    "tiktok_pop": "Style: Default,Arial Black,80,&H0000FFFF,&H0000FFFF,&H00000000,&H00000000,-1,0,0,0,100,100,0,0,1,6,0,2,80,80,210,1",
    "karaoke_pop": "Style: Default,Arial Black,80,&H0000FFFF,&H00FFFFFF,&H00000000,&H00000000,-1,0,0,0,100,100,0,0,1,6,0,2,80,80,210,1",
    "subtitle_bar": "Style: Default,Arial,64,&H00FFFFFF,&H0000FFFF,&H00000000,&H80000000,-1,0,0,0,100,100,0,0,3,0,0,2,80,80,210,1",
    "gaming": "Style: Default,Impact,80,&H00FFFFFF,&H0000FFFF,&H00000000,&H00000000,-1,0,0,0,100,100,0,0,1,8,2,2,80,80,210,1",
}


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
) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    style_str = STYLE_LINES.get(style_preset, STYLE_LINES["default"])
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
        if end <= start:
            end = start + 0.6

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
                            f"Dialogue: 0,{ass_time(c_start)},{ass_time(c_end)},Default,,0,0,0,,{karaoke}"
                        )
                continue
            dialogue_text = format_karaoke_text(words_raw) or ass_escape_text(text)
        else:
            dialogue_text = ass_escape_text(text)

        if not dialogue_text:
            continue
        lines.append(
            f"Dialogue: 0,{ass_time(start)},{ass_time(end)},Default,,0,0,0,,{dialogue_text}"
        )

    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


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
