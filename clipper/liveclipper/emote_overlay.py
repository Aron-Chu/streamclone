from __future__ import annotations

import urllib.request
from pathlib import Path
from typing import Any


def download_emote_image(url: str, dest: Path) -> bool:
    if not url or dest.exists():
        return dest.exists()
    try:
        dest.parent.mkdir(parents=True, exist_ok=True)
        req = urllib.request.Request(url, headers={"User-Agent": "StreamcloneClipper/1.0"})
        with urllib.request.urlopen(req, timeout=15) as resp:
            dest.write_bytes(resp.read())
        return dest.stat().st_size > 0
    except Exception:
        return False


def prepare_emote_assets(
    emote_map: dict[str, str],
    temp_dir: Path,
) -> dict[str, Path]:
    assets: dict[str, Path] = {}
    for name, url in emote_map.items():
        key = name.strip().lower()
        if not key or not url:
            continue
        safe = "".join(ch if ch.isalnum() else "_" for ch in key)
        dest = temp_dir / f"emote_{safe}.png"
        if download_emote_image(url, dest):
            assets[key] = dest
    return assets


def prepare_top_emote_assets(
    top_emotes: list[dict[str, Any]],
    temp_dir: Path,
) -> dict[str, Path]:
    assets: dict[str, Path] = {}
    for emote in top_emotes[:5]:
        name = str(emote.get("name") or "").strip().lower()
        url = str(emote.get("image_url") or emote.get("imageUrl") or "")
        if not name or not url:
            continue
        safe = "".join(ch if ch.isalnum() else "_" for ch in name)
        dest = temp_dir / f"strip_{safe}.png"
        if download_emote_image(url, dest):
            assets[name] = dest
    return assets


def build_reaction_strip_filter(
    *,
    input_label: str,
    output_label: str,
    top_emotes: list[dict[str, Any]],
    assets: dict[str, Path],
    width: int = 1080,
) -> str:
    items: list[tuple[str, int]] = []
    for emote in top_emotes[:5]:
        name = str(emote.get("name") or "").strip().lower()
        if not name:
            continue
        path = assets.get(name)
        if path:
            count = int(emote.get("count") or 0)
            items.append((str(path).replace("\\", "/").replace(":", "\\:"), count))

    if not items:
        return f"[{input_label}]null[{output_label}]"

    emote_w = 72
    gap = 12
    total_w = len(items) * emote_w + max(0, len(items) - 1) * gap
    start_x = max(20, (width - total_w) // 2)
    chain = f"[{input_label}]"
    current = input_label
    for idx, (path, _) in enumerate(items):
        emote_label = f"ere{idx}"
        out_label = f"erv{idx}"
        x = start_x + idx * (emote_w + gap)
        chain += f"movie='{path}'[{emote_label}];"
        scaled = f"es{idx}"
        chain += f"[{emote_label}]scale={emote_w}:{emote_w}:flags=lanczos[{scaled}];"
        chain += f"[{current}][{scaled}]overlay={x}:36:enable='gte(t\\,0)'[{out_label}];"
        current = out_label
    chain += f"[{current}]null[{output_label}]"
    return chain


def build_caption_emote_overlays(
    *,
    input_label: str,
    output_label: str,
    emote_hits: list[dict[str, Any]],
    assets: dict[str, Path],
    width: int = 1080,
    height: int = 1920,
) -> str:
    if not emote_hits or not assets:
        return f"[{input_label}]null[{output_label}]"

    emote_w = 56
    y = int(height * 0.72)
    chain = f"[{input_label}]"
    current = input_label
    seen = 0
    for hit in emote_hits:
        name = str(hit.get("name") or "").strip().lower()
        path = assets.get(name)
        if not path:
            continue
        start = float(hit.get("start") or 0)
        end = float(hit.get("end") or start + 0.8)
        emote_label = f"ce{seen}"
        out_label = f"cv{seen}"
        x = (width - emote_w) // 2
        escaped = str(path).replace("\\", "/").replace(":", "\\:")
        scaled = f"cs{seen}"
        chain += f"movie='{escaped}'[{emote_label}];"
        chain += f"[{emote_label}]scale={emote_w}:{emote_w}:flags=lanczos[{scaled}];"
        chain += (
            f"[{current}][{scaled}]overlay={x}:{y}:"
            f"enable='between(t\\,{start:.3f}\\,{end:.3f})'[{out_label}];"
        )
        current = out_label
        seen += 1
        if seen >= 8:
            break
    chain += f"[{current}]null[{output_label}]"
    return chain
