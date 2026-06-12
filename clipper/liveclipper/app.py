from __future__ import annotations

import html
from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException, Request, BackgroundTasks
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, HTMLResponse, JSONResponse

from .cleanup import CleanupService
from .config import ensure_dirs, load_config
from .db import Store
from .irc import IRCMonitor, Spike, VelocityDetector
from .render import Renderer
from .security import check_webhook_token
from .streamlink import StreamlinkDownloader
from .timeutil import iso_from_ms, now_ms
from .templates import TemplateLoader, resolve_render_options
from .transcribe import Transcriber, offset_caption_segments, write_ass
from .twitch import TwitchClient
from .validate import normalize_channel
from .worker import JobWorker


def _probe_twitch_token(client_id: str, access_token: str) -> dict[str, object]:
    import json
    import urllib.error
    import urllib.request

    if not access_token:
        return {"ok": False, "reason": "token_missing"}
    req = urllib.request.Request(
        "https://id.twitch.tv/oauth2/validate",
        headers={"Authorization": "Bearer " + access_token.strip()},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            body = json.loads(resp.read().decode())
            token_client = str(body.get("client_id") or "")
            return {
                "ok": True,
                "client_id_match": token_client == client_id,
                "token_client_prefix": token_client[:4] if len(token_client) >= 4 else "",
                "has_clips_edit": "clips:edit" in (body.get("scopes") or []),
                "expires_in": body.get("expires_in"),
            }
    except urllib.error.HTTPError as exc:
        return {
            "ok": False,
            "reason": "validate_http_error",
            "status": exc.code,
            "body_excerpt": exc.read().decode("utf-8", errors="replace")[:160],
        }
    except Exception as exc:
        return {"ok": False, "reason": "validate_failed", "error": str(exc)[:160]}


def create_app() -> FastAPI:
    cfg = load_config()
    ensure_dirs(cfg)
    store = Store(cfg.db_path)
    store.init()
    twitch = TwitchClient(cfg.twitch_api_url, cfg.twitch_client_id, cfg.twitch_user_access_token)
    downloader = StreamlinkDownloader(cfg.twitch_user_access_token, cfg.streamlink_bin)
    transcriber = Transcriber(
        cfg.whisper_model,
        cfg.whisper_compute_type,
        cfg.asr_enabled,
        cfg.asr_required,
    )
    renderer = Renderer(cfg.ffmpeg_encoder, cfg.ffmpeg_preset, cfg.ffmpeg_bin)
    template_loader = TemplateLoader()
    worker = JobWorker(
        cfg=cfg,
        store=store,
        twitch=twitch,
        downloader=downloader,
        transcriber=transcriber,
        renderer=renderer,
    )

    def perform_rerender(
        job_id: str,
        trim_start: float | None,
        trim_duration: float | None,
        format_preset: str | None,
        caption_preset: str | None,
        template_id: str | None,
        caption_size: str | None = None,
        caption_position: str | None = None,
        layout: str | None = None,
        layout_split_ratio: float | None = None,
        emote_map: dict[str, str] | None = None,
        preview_only: bool = False,
    ):
        try:
            job = store.get_job(job_id)
            if not job:
                return

            raw_path = Path(job.get("raw_path") or "")
            if not raw_path.exists():
                store.set_state(job_id, "failed", f"re-render failed: raw source file missing at {raw_path}")
                return

            resolved = resolve_render_options(
                template_id=template_id,
                format_preset=format_preset,
                caption_preset=caption_preset,
                loader=template_loader,
            )

            job_dir = cfg.output_dir / "tmp" / (job_id + "_render")
            job_dir.mkdir(parents=True, exist_ok=True)
            captions_path = job_dir / "captions.ass"

            import json
            import shutil
            try:
                captions_list = json.loads(job.get("captions") or "[]")
            except Exception:
                captions_list = []

            effective_trim = float(trim_start or 0.0)
            final_dur = trim_duration or float(job.get("final_duration") or cfg.final_duration)
            trim_end = effective_trim + final_dur if effective_trim > 0 else None
            offset_segments = offset_caption_segments(
                captions_list,
                trim_start=effective_trim,
                trim_end=trim_end,
            )

            emote_hits: list[dict[str, Any]] = []
            emote_names = {k.strip().lower() for k in (emote_map or {})}
            if resolved.caption_preset == "none":
                captions_file = None
            else:
                emote_hits = write_ass(
                    captions_path,
                    offset_segments,
                    style_preset=resolved.caption_preset,
                    max_words_per_line=resolved.max_words_per_line,
                    caption_size=caption_size or "md",
                    caption_position=caption_position or "bottom",
                    emote_names=emote_names or None,
                )
                captions_file = captions_path

            moment_context = job.get("moment_context")
            if isinstance(moment_context, str):
                try:
                    moment_context = json.loads(moment_context)
                except Exception:
                    moment_context = None

            final_path = Path(job.get("final_path") or (str(cfg.output_dir / (job_id + ".mp4"))))
            if preview_only:
                final_path = cfg.output_dir / "tmp" / f"{job_id}_preview.mp4"

            if not preview_only:
                store.set_state(job_id, "rendering", "re-rendering video")

            source_duration = job.get("twitch_clip_duration") or float(job.get("source_duration") or cfg.source_duration)

            active_layout = layout or resolved.video_effects.layout
            split_ratio = float(layout_split_ratio if layout_split_ratio is not None else 0.35)

            renderer.render(
                input_path=raw_path,
                output_path=final_path,
                captions_path=captions_file,
                source_duration=source_duration,
                final_duration=final_dur,
                event_latency_offset=float(job.get("event_latency_offset") or cfg.event_latency_offset),
                trigger_detected_at_ms=int(job.get("trigger_detected_at") or 0),
                peak_chat_ts_ms=job.get("peak_chat_ts"),
                format_preset=resolved.format_preset,
                trim_start=trim_start,
                video_effects=resolved.video_effects,
                audio_effects=resolved.audio_effects,
                layout=active_layout,
                layout_split_ratio=split_ratio,
                emote_assets_dir=job_dir,
                emote_map=emote_map,
                emote_hits=emote_hits,
                moment_context=moment_context if isinstance(moment_context, dict) else None,
                preview_mode=preview_only,
            )

            if preview_only:
                store.update_job(job_id, preview_path=str(final_path))
                return

            store.set_state(
                job_id,
                "ready",
                "re-render final version ready",
                final_path=str(final_path),
                artifact_available=1,
            )
            shutil.rmtree(job_dir, ignore_errors=True)
        except Exception as exc:
            if preview_only:
                store.add_warning(job_id, f"preview render failed: {exc}")
                return
            store.set_state(job_id, "failed", f"re-render failed: {exc}")

    def perform_retranscribe(
        job_id: str,
        trim_start: float,
        trim_duration: float | None,
        max_words_per_line: int,
    ):
        try:
            job = store.get_job(job_id)
            if not job:
                return
            raw_path = Path(job.get("raw_path") or "")
            if not raw_path.exists():
                store.set_state(job_id, "failed", f"re-transcribe failed: raw source file missing at {raw_path}")
                return

            job_dir = cfg.output_dir / "tmp" / (job_id + "_asr")
            job_dir.mkdir(parents=True, exist_ok=True)
            captions_path = job_dir / "captions.ass"

            store.set_state(job_id, "transcribing", "re-transcribing trimmed region")
            result = transcriber.transcribe(
                raw_path,
                captions_path,
                trim_start=trim_start,
                trim_duration=trim_duration,
                max_words_per_line=max_words_per_line,
            )
            if result.warning:
                store.add_warning(job_id, result.warning)
            import json
            if result.segments:
                store.update_job(
                    job_id,
                    captions_path=str(result.path) if result.path else None,
                    captions=json.dumps(result.segments),
                )
                next_state = "ready" if job.get("artifact_available") else (job.get("state") or "ready")
                store.set_state(job_id, next_state, "captions updated from re-transcription")
            else:
                store.set_state(job_id, "failed", "re-transcribe produced no captions")
        except Exception as exc:
            store.set_state(job_id, "failed", f"re-transcribe failed: {exc}")

    def queue_spike(spike: Spike) -> None:
        store.insert_job(
            channel=spike.channel,
            broadcaster_id="",
            trigger_type="chat_spike",
            reason=spike.reason,
            title=f"{spike.channel} chat spike",
            requested_duration=cfg.source_duration,
            source_duration=cfg.source_duration,
            final_duration=cfg.final_duration,
            event_latency_offset=cfg.event_latency_offset,
            trigger_detected_at=now_ms(),
            peak_chat_ts=spike.peak_chat_ts,
            message_count=spike.message_count,
            duplicate_window_seconds=cfg.duplicate_window_seconds,
        )
        worker.wake()

    detector = VelocityDetector(
        window_seconds=cfg.chat_window_seconds,
        min_messages=cfg.chat_min_messages,
        multiplier=cfg.chat_spike_multiplier,
        cooldown_seconds=cfg.cooldown_seconds,
    )
    irc = IRCMonitor(
        irc_url=cfg.twitch_irc_url,
        detector=detector,
        trigger=queue_spike,
        note_error=store.note_channel_error,
    )
    cleanup = CleanupService(
        store,
        cfg.output_dir,
        cfg.final_retention_hours,
        cfg.cleanup_interval_seconds,
    )

    app = FastAPI(title="Streamclone Live Clipper", version="0.1.0")
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )
    app.state.cfg = cfg
    app.state.store = store
    app.state.worker = worker
    app.state.irc = irc
    app.state.cleanup = cleanup

    @app.on_event("startup")
    def startup() -> None:
        for channel in store.list_watched_channels():
            if channel.get("enabled"):
                irc.watch(channel["login"])
        worker.start()
        irc.start()
        cleanup.run_once()
        cleanup.start()

    @app.on_event("shutdown")
    def shutdown() -> None:
        cleanup.stop()
        irc.stop()
        worker.stop()
        store.close()

    @app.get("/healthz")
    def healthz() -> dict[str, str]:
        return {"status": "ok", "service": "clipper"}

    @app.get("/", response_class=JSONResponse)
    def api_root() -> dict[str, str]:
        return {"status": "ok", "service": "Streamclone Clipper Service"}

    @app.get("/v1/twitch/status")
    def twitch_status() -> dict[str, Any]:
        if not cfg.twitch_client_id or not cfg.twitch_user_access_token:
            return {
                "ok": False,
                "failure_code": "twitch_not_configured",
                "remediation": (
                    "Set TWITCH_OAUTH_CLIENT_ID and run make twitch-local-auth to write "
                    "CLIPPER_TWITCH_USER_ACCESS_TOKEN, then recreate the clipper service."
                ),
            }
        probe = _probe_twitch_token(cfg.twitch_client_id, cfg.twitch_user_access_token)
        if probe.get("ok"):
            if probe.get("client_id_match") is False:
                return {
                    "ok": False,
                    "failure_code": "client_id_mismatch",
                    "remediation": (
                        "CLIPPER_TWITCH_CLIENT_ID does not match the Twitch app that issued the token. "
                        "Run make twitch-local-auth to resync both values."
                    ),
                }
            if not probe.get("has_clips_edit"):
                return {
                    "ok": False,
                    "failure_code": "missing_scope",
                    "remediation": (
                        "Twitch token is missing clips:edit. Run make twitch-local-auth and approve "
                        "the login prompt, then recreate the clipper service."
                    ),
                }
            return {
                "ok": True,
                "expires_in": probe.get("expires_in"),
                "has_clips_edit": True,
            }
        failure_code = "invalid_token"
        if probe.get("reason") == "token_missing":
            failure_code = "twitch_not_configured"
        return {
            "ok": False,
            "failure_code": failure_code,
            "remediation": (
                "Twitch token is expired or revoked. Run make twitch-local-auth, approve the Twitch "
                "login in your browser, then recreate the clipper service. Restarting clipper alone "
                "does not refresh the token."
            ),
        }

    @app.get("/v1/channels")
    def list_channels() -> dict[str, Any]:
        return {"items": store.list_watched_channels()}

    @app.post("/v1/channels/{login}/watch")
    async def watch_channel(login: str, request: Request) -> JSONResponse:
        check_webhook_token(request, cfg.webhook_token)
        channel = normalize_channel(login)
        body = await json_body(request)
        row = store.upsert_watched_channel(channel, str(body.get("broadcaster_id") or ""))
        irc.watch(channel)
        return JSONResponse({"channel": row})

    @app.delete("/v1/channels/{login}/watch")
    async def unwatch_channel(login: str, request: Request) -> dict[str, str]:
        check_webhook_token(request, cfg.webhook_token)
        channel = normalize_channel(login)
        store.disable_watched_channel(channel)
        irc.unwatch(channel)
        return {"status": "ok"}

    @app.post("/v1/triggers/streamerbot")
    async def streamerbot_trigger(request: Request) -> JSONResponse:
        check_webhook_token(request, cfg.webhook_token)
        body = await json_body(request)
        return enqueue_trigger(body, "streamerbot")

    @app.post("/v1/triggers/manual")
    async def manual_trigger(request: Request) -> JSONResponse:
        check_webhook_token(request, cfg.webhook_token)
        body = await json_body(request)
        return enqueue_trigger(body, "manual")

    @app.get("/v1/jobs")
    def list_jobs(limit: int = 100, channel: str | None = None) -> dict[str, Any]:
        limit = max(1, min(limit, 500))
        return {"items": store.list_jobs(limit, channel)}

    @app.get("/v1/jobs/{job_id}")
    def get_job(job_id: str) -> dict[str, Any]:
        job = store.get_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})
        return {"job": job, "events": store.list_events(job_id)}

    @app.post("/v1/jobs/{job_id}/retry")
    async def retry_job(job_id: str, request: Request) -> dict[str, Any]:
        check_webhook_token(request, cfg.webhook_token)
        job = store.retry_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})
        worker.wake()
        return {"job": job}

    @app.get("/v1/jobs/{job_id}/final.mp4")
    def final_mp4(job_id: str) -> FileResponse:
        job = store.get_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})
        if not job.get("artifact_available") or not job.get("final_path"):
            raise HTTPException(status_code=404, detail={"error": "artifact_not_available"})
        path = Path(job["final_path"])
        if not path.exists():
            store.update_job(job_id, artifact_available=0)
            raise HTTPException(status_code=404, detail={"error": "artifact_missing"})
        return FileResponse(path, media_type="video/mp4", filename=path.name)

    @app.get("/v1/jobs/{job_id}/source.mp4")
    def source_mp4(job_id: str) -> FileResponse:
        job = store.get_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})
        if not job.get("raw_path"):
            raise HTTPException(status_code=404, detail={"error": "raw_path_not_available"})
        path = Path(job["raw_path"])
        if not path.exists():
            raise HTTPException(status_code=404, detail={"error": "source_missing"})
        return FileResponse(path, media_type="video/mp4", filename=path.name)

    @app.get("/v1/templates")
    def list_templates() -> dict[str, Any]:
        return {"items": template_loader.list_public()}

    @app.get("/v1/jobs/{job_id}/captions")
    def get_captions(job_id: str) -> dict[str, Any]:
        job = store.get_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})
        import json
        try:
            captions_data = json.loads(job.get("captions") or "[]")
        except Exception:
            captions_data = []
        return {"captions": captions_data}

    @app.put("/v1/jobs/{job_id}/captions")
    async def update_captions(job_id: str, request: Request) -> dict[str, Any]:
        check_webhook_token(request, cfg.webhook_token)
        job = store.get_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})
        body = await json_body(request)
        captions_list = body.get("captions")
        if not isinstance(captions_list, list):
            raise HTTPException(status_code=400, detail={"error": "captions_must_be_list"})
        import json
        store.update_job(job_id, captions=json.dumps(captions_list))
        return {"status": "ok"}

    @app.post("/v1/jobs/{job_id}/render")
    async def render_job(
        job_id: str,
        background_tasks: BackgroundTasks,
        request: Request,
    ) -> dict[str, Any]:
        check_webhook_token(request, cfg.webhook_token)
        job = store.get_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})

        body = await json_body(request)
        trim_start = body.get("trim_start")
        if trim_start is not None:
            trim_start = float(trim_start)

        trim_duration = body.get("trim_duration") or body.get("final_duration")
        if trim_duration is not None:
            trim_duration = float(trim_duration)

        format_preset = body.get("format_preset")
        if format_preset is not None:
            format_preset = str(format_preset)
        caption_preset = body.get("caption_preset")
        if caption_preset is not None:
            caption_preset = str(caption_preset)
        template_id = body.get("template_id")
        if template_id is not None:
            template_id = str(template_id)

        caption_size = body.get("caption_size")
        if caption_size is not None:
            caption_size = str(caption_size)
        caption_position = body.get("caption_position")
        if caption_position is not None:
            caption_position = str(caption_position)
        layout = body.get("layout")
        if layout is not None:
            layout = str(layout)
        layout_split_ratio = body.get("layout_split_ratio")
        if layout_split_ratio is not None:
            layout_split_ratio = float(layout_split_ratio)
        emote_map_raw = body.get("emote_map")
        emote_map: dict[str, str] | None = None
        if isinstance(emote_map_raw, dict):
            emote_map = {str(k): str(v) for k, v in emote_map_raw.items() if v}

        store.set_state(job_id, "rendering", "queued for re-render")

        background_tasks.add_task(
            perform_rerender,
            job_id=job_id,
            trim_start=trim_start,
            trim_duration=trim_duration,
            format_preset=format_preset,
            caption_preset=caption_preset,
            template_id=template_id,
            caption_size=caption_size,
            caption_position=caption_position,
            layout=layout,
            layout_split_ratio=layout_split_ratio,
            emote_map=emote_map,
        )
        return {"status": "rendering", "job_id": job_id}

    @app.post("/v1/jobs/{job_id}/transcribe")
    async def transcribe_job(
        job_id: str,
        background_tasks: BackgroundTasks,
        request: Request,
    ) -> dict[str, Any]:
        check_webhook_token(request, cfg.webhook_token)
        job = store.get_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})

        body = await json_body(request)
        trim_start = float(body.get("trim_start") or 0.0)
        trim_duration = body.get("trim_duration")
        if trim_duration is not None:
            trim_duration = float(trim_duration)
        max_words_per_line = int(body.get("max_words_per_line") or 3)

        background_tasks.add_task(
            perform_retranscribe,
            job_id=job_id,
            trim_start=trim_start,
            trim_duration=trim_duration,
            max_words_per_line=max_words_per_line,
        )
        return {"status": "transcribing", "job_id": job_id}

    @app.get("/v1/jobs/{job_id}/project")
    def get_project(job_id: str) -> dict[str, Any]:
        job = store.get_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})
        import json
        raw = job.get("editor_project") or "{}"
        try:
            project = json.loads(raw) if isinstance(raw, str) else raw
        except Exception:
            project = {}
        if not isinstance(project, dict):
            project = {}
        return {"project": project}

    @app.put("/v1/jobs/{job_id}/project")
    async def put_project(job_id: str, request: Request) -> dict[str, str]:
        check_webhook_token(request, cfg.webhook_token)
        job = store.get_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})
        body = await json_body(request)
        project = body.get("project")
        if not isinstance(project, dict):
            raise HTTPException(status_code=400, detail={"error": "project_must_be_object"})
        import json
        store.update_job(job_id, editor_project=json.dumps(project))
        return {"status": "ok"}

    @app.post("/v1/jobs/{job_id}/preview")
    async def preview_job(
        job_id: str,
        background_tasks: BackgroundTasks,
        request: Request,
    ) -> dict[str, str]:
        check_webhook_token(request, cfg.webhook_token)
        job = store.get_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})
        body = await json_body(request)
        background_tasks.add_task(
            perform_rerender,
            job_id=job_id,
            trim_start=float(body.get("trim_start") or 0.0),
            trim_duration=float(body.get("trim_duration") or job.get("final_duration") or cfg.final_duration),
            format_preset=str(body.get("format_preset") or "tiktok"),
            caption_preset=str(body.get("caption_preset") or "default"),
            template_id=body.get("template_id"),
            caption_size=body.get("caption_size"),
            caption_position=body.get("caption_position"),
            layout=body.get("layout"),
            layout_split_ratio=body.get("layout_split_ratio"),
            emote_map=body.get("emote_map") if isinstance(body.get("emote_map"), dict) else None,
            preview_only=True,
        )
        return {"status": "preview_queued", "job_id": job_id}

    @app.get("/v1/jobs/{job_id}/preview.mp4")
    def preview_mp4(job_id: str) -> FileResponse:
        job = store.get_job(job_id)
        if not job:
            raise HTTPException(status_code=404, detail={"error": "job_not_found"})
        path_raw = job.get("preview_path")
        if not path_raw:
            raise HTTPException(status_code=404, detail={"error": "preview_not_available"})
        path = Path(str(path_raw))
        if not path.exists():
            raise HTTPException(status_code=404, detail={"error": "preview_missing"})
        return FileResponse(path, media_type="video/mp4", filename=path.name)

    @app.post("/v1/jobs/batch")
    async def batch_queue(request: Request) -> dict[str, Any]:
        check_webhook_token(request, cfg.webhook_token)
        body = await json_body(request)
        moments = body.get("moments")
        if not isinstance(moments, list) or not moments:
            raise HTTPException(status_code=400, detail={"error": "moments_required"})
        queued: list[str] = []
        suppressed: list[str] = []
        for item in moments[:20]:
            if not isinstance(item, dict):
                continue
            result = enqueue_trigger_body(item, str(item.get("trigger_type") or "manual"))
            if result.get("status") == "queued" and result.get("job_id"):
                queued.append(str(result["job_id"]))
            elif result.get("existing_job_id"):
                suppressed.append(str(result["existing_job_id"]))
        return {"queued": queued, "suppressed": suppressed, "count": len(queued)}

    def enqueue_trigger_body(body: dict[str, Any], trigger_type: str) -> dict[str, Any]:
        try:
            channel = normalize_channel(str(body.get("channel") or body.get("streamer") or ""))
        except ValueError:
            return {"status": "error", "error": "invalid_channel"}
        duration = float(body.get("duration") or cfg.source_duration)
        final_duration = float(body.get("final_duration") or cfg.final_duration)
        import json as json_mod
        moment_context_raw = body.get("moment_context")
        moment_context_value: str | None = None
        if isinstance(moment_context_raw, dict):
            moment_context_value = json_mod.dumps(moment_context_raw)
        elif isinstance(moment_context_raw, str) and moment_context_raw.strip():
            moment_context_value = moment_context_raw.strip()

        result = store.insert_job(
            channel=channel,
            broadcaster_id=str(body.get("broadcaster_id") or ""),
            trigger_type=trigger_type,
            reason=str(body.get("reason") or trigger_type),
            title=str(body.get("title") or f"{channel} clip"),
            requested_duration=duration,
            source_duration=min(max(duration, 5.0), 60.0),
            final_duration=min(max(final_duration, 5.0), 60.0),
            event_latency_offset=float(body.get("event_latency_offset") or cfg.event_latency_offset),
            trigger_detected_at=int(body.get("trigger_detected_at") or now_ms()),
            peak_chat_ts=as_optional_int(body.get("peak_chat_ts")),
            message_count=as_optional_int(body.get("message_count")),
            duplicate_window_seconds=cfg.duplicate_window_seconds,
            moment_context=moment_context_value,
        )
        if result.suppressed:
            return {
                "status": "suppressed",
                "reason": result.reason,
                "existing_job_id": result.existing_job_id,
            }
        worker.wake()
        return {"status": "queued", "job_id": result.job_id}

    def enqueue_trigger(body: dict[str, Any], trigger_type: str) -> JSONResponse:
        payload = enqueue_trigger_body(body, trigger_type)
        if payload.get("status") == "suppressed":
            return JSONResponse(payload, status_code=202)
        if payload.get("status") == "error":
            raise HTTPException(status_code=400, detail={"error": payload.get("error")})
        return JSONResponse(payload, status_code=202)

    return app


async def json_body(request: Request) -> dict[str, Any]:
    try:
        body = await request.json()
    except Exception:
        return {}
    return body if isinstance(body, dict) else {}


def as_optional_int(value: Any) -> int | None:
    if value in {None, ""}:
        return None
    return int(value)
