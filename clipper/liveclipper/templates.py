from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


@dataclass(frozen=True)
class IntroZoom:
    start: float = 0.0
    duration: float = 0.4
    scale: float = 1.08


@dataclass(frozen=True)
class VideoEffects:
    layout: str = "blur_bg_center"
    intro_zoom: IntroZoom | None = None
    vignette: bool = False


@dataclass(frozen=True)
class CaptionConfig:
    preset: str = "default"
    max_words_per_line: int = 3
    position: str = "bottom"


@dataclass(frozen=True)
class AudioEffects:
    noise_reduce: str | None = None
    normalize: bool = False


@dataclass(frozen=True)
class EditRecipe:
    id: str
    name: str
    description: str = ""
    format_preset: str = "tiktok"
    caption: CaptionConfig = field(default_factory=CaptionConfig)
    video: VideoEffects = field(default_factory=VideoEffects)
    audio: AudioEffects = field(default_factory=AudioEffects)


@dataclass(frozen=True)
class ResolvedRenderOptions:
    format_preset: str
    caption_preset: str
    max_words_per_line: int
    video_effects: VideoEffects
    audio_effects: AudioEffects
    template_id: str | None = None
    template_name: str | None = None


def default_templates_dir() -> Path:
    return Path(__file__).resolve().parent.parent / "templates"


def _parse_intro_zoom(raw: dict[str, Any] | None) -> IntroZoom | None:
    if not raw:
        return None
    return IntroZoom(
        start=float(raw.get("start", 0)),
        duration=float(raw.get("duration", 0.4)),
        scale=float(raw.get("scale", 1.08)),
    )


def _parse_recipe(data: dict[str, Any]) -> EditRecipe:
    caption_raw = data.get("caption") or {}
    video_raw = data.get("video") or {}
    audio_raw = data.get("audio") or {}
    return EditRecipe(
        id=str(data["id"]),
        name=str(data.get("name") or data["id"]),
        description=str(data.get("description") or ""),
        format_preset=str(data.get("format_preset") or "tiktok"),
        caption=CaptionConfig(
            preset=str(caption_raw.get("preset") or "default"),
            max_words_per_line=int(caption_raw.get("max_words_per_line") or 3),
            position=str(caption_raw.get("position") or "bottom"),
        ),
        video=VideoEffects(
            layout=str(video_raw.get("layout") or "blur_bg_center"),
            intro_zoom=_parse_intro_zoom(video_raw.get("intro_zoom")),
            vignette=bool(video_raw.get("vignette")),
        ),
        audio=AudioEffects(
            noise_reduce=audio_raw.get("noise_reduce"),
            normalize=bool(audio_raw.get("normalize")),
        ),
    )


class TemplateLoader:
    def __init__(self, templates_dir: Path | None = None):
        self.templates_dir = templates_dir or default_templates_dir()
        self._cache: dict[str, EditRecipe] | None = None

    def reload(self) -> None:
        self._cache = None

    def load_all(self) -> dict[str, EditRecipe]:
        if self._cache is not None:
            return self._cache
        recipes: dict[str, EditRecipe] = {}
        if not self.templates_dir.exists():
            self._cache = recipes
            return recipes
        for path in sorted(self.templates_dir.glob("*.json")):
            try:
                data = json.loads(path.read_text(encoding="utf-8"))
                recipe = _parse_recipe(data)
                recipes[recipe.id] = recipe
            except Exception:
                continue
        self._cache = recipes
        return recipes

    def get(self, template_id: str) -> EditRecipe | None:
        return self.load_all().get(template_id)

    def list_public(self) -> list[dict[str, Any]]:
        items = []
        for recipe in self.load_all().values():
            items.append(
                {
                    "id": recipe.id,
                    "name": recipe.name,
                    "description": recipe.description,
                    "format_preset": recipe.format_preset,
                    "caption_preset": recipe.caption.preset,
                    "max_words_per_line": recipe.caption.max_words_per_line,
                    "has_intro_zoom": recipe.video.intro_zoom is not None,
                    "has_vignette": recipe.video.vignette,
                    "noise_reduce": recipe.audio.noise_reduce,
                }
            )
        return items


def resolve_render_options(
    *,
    template_id: str | None,
    format_preset: str | None,
    caption_preset: str | None,
    loader: TemplateLoader | None = None,
) -> ResolvedRenderOptions:
    loader = loader or TemplateLoader()
    recipe = loader.get(template_id) if template_id else None
    if recipe:
        return ResolvedRenderOptions(
            format_preset=format_preset or recipe.format_preset,
            caption_preset=caption_preset or recipe.caption.preset,
            max_words_per_line=recipe.caption.max_words_per_line,
            video_effects=recipe.video,
            audio_effects=recipe.audio,
            template_id=recipe.id,
            template_name=recipe.name,
        )
    return ResolvedRenderOptions(
        format_preset=format_preset or "tiktok",
        caption_preset=caption_preset or "default",
        max_words_per_line=3,
        video_effects=VideoEffects(),
        audio_effects=AudioEffects(),
        template_id=None,
        template_name=None,
    )
