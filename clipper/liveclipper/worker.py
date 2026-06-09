from __future__ import annotations

import shutil
import threading
from pathlib import Path

from .config import Config
from .db import Store
from .render import RenderError, Renderer
from .streamlink import DownloadError, StreamlinkDownloader
from .transcribe import Transcriber, TranscriptionError
from .twitch import TwitchClient, TwitchError


class JobWorker:
    def __init__(
        self,
        *,
        cfg: Config,
        store: Store,
        twitch: TwitchClient,
        downloader: StreamlinkDownloader,
        transcriber: Transcriber,
        renderer: Renderer,
    ):
        self.cfg = cfg
        self.store = store
        self.twitch = twitch
        self.downloader = downloader
        self.transcriber = transcriber
        self.renderer = renderer
        self._wake = threading.Event()
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None

    def start(self) -> None:
        if self._thread and self._thread.is_alive():
            return
        self._thread = threading.Thread(target=self._loop, name="clipper-worker", daemon=True)
        self._thread.start()

    def stop(self) -> None:
        self._stop.set()
        self._wake.set()
        if self._thread:
            self._thread.join(timeout=5)

    def wake(self) -> None:
        self._wake.set()

    def _loop(self) -> None:
        while not self._stop.is_set():
            job = self.store.claim_next_job(self.cfg.stale_job_seconds * 1000)
            if not job:
                self._wake.wait(2)
                self._wake.clear()
                continue
            self._process(job)

    def _process(self, job: dict) -> None:
        job_id = job["id"]
        job_dir = self.cfg.output_dir / "tmp" / job_id
        raw_path = Path(job.get("raw_path") or self.cfg.output_dir / (job_id + "_raw.mp4"))
        captions_path = Path(job["captions_path"]) if job.get("captions_path") else job_dir / "captions.ass"
        final_path = self.cfg.output_dir / (job_id + ".mp4")
        try:
            if final_path.exists():
                self.store.set_state(
                    job_id,
                    "ready",
                    "final render ready",
                    final_path=str(final_path),
                    artifact_available=1,
                )
                shutil.rmtree(job_dir, ignore_errors=True)
                return
            if raw_path.exists():
                source_duration = float(
                    job.get("twitch_clip_duration")
                    or job.get("source_duration")
                    or self.cfg.source_duration
                )
                self._finish_from_raw(job_id, job, raw_path, captions_path, final_path, source_duration)
                return
            if job.get("twitch_clip_url"):
                self.store.set_state(job_id, "downloading", "resuming clip download")
                self.downloader.download(str(job["twitch_clip_url"]), raw_path)
                self.store.update_job(job_id, raw_path=str(raw_path))
                source_duration = float(
                    job.get("twitch_clip_duration")
                    or job.get("source_duration")
                    or self.cfg.source_duration
                )
                self._finish_from_raw(job_id, job, raw_path, captions_path, final_path, source_duration)
                return

            broadcaster_id = job.get("broadcaster_id") or ""
            if not broadcaster_id:
                broadcaster_id = self.twitch.resolve_broadcaster_id(job["channel"])
                self.store.update_job(job_id, broadcaster_id=broadcaster_id)

            created = self.twitch.create_clip(
                broadcaster_id=broadcaster_id,
                title=job.get("title") or "",
                duration=float(job.get("source_duration") or self.cfg.source_duration),
            )
            self.store.update_job(job_id, twitch_clip_id=created.clip_id, twitch_edit_url=created.edit_url)
            self.store.set_state(job_id, "waiting_for_clip", "clip created; polling readiness")
            ready = self.twitch.poll_clip(
                created.clip_id,
                self.cfg.clip_poll_timeout_seconds,
                self.cfg.clip_poll_interval_seconds,
            )
            self.store.update_job(
                job_id,
                twitch_clip_url=ready.url,
                twitch_clip_duration=ready.duration,
            )

            self.store.set_state(job_id, "downloading", "downloading clip")
            self.downloader.download(ready.url, raw_path)
            self.store.update_job(job_id, raw_path=str(raw_path))

            source_duration = ready.duration or float(job.get("source_duration") or self.cfg.source_duration)
            self._finish_from_raw(job_id, job, raw_path, captions_path, final_path, source_duration)
        except TwitchError as exc:
            self._fail(job_id, exc.code, exc.message)
        except DownloadError as exc:
            self._fail(job_id, exc.code, exc.message)
        except TranscriptionError as exc:
            self._fail(job_id, "transcribe_failed", str(exc))
        except RenderError as exc:
            self._fail(job_id, "render_failed", str(exc))
        except Exception as exc:
            self._fail(job_id, "job_failed", str(exc))

    def _finish_from_raw(
        self,
        job_id: str,
        job: dict,
        raw_path: Path,
        captions_path: Path,
        final_path: Path,
        source_duration: float,
    ) -> None:
        job_dir = self.cfg.output_dir / "tmp" / job_id
        caption_file: Path | None = Path(job["captions_path"]) if job.get("captions_path") else None
        if caption_file and not caption_file.exists():
            caption_file = None
        if self.cfg.asr_enabled and caption_file is None and not captions_path.exists():
            self.store.set_state(job_id, "transcribing", "transcribing audio")
            result = self.transcriber.transcribe(raw_path, captions_path)
            if result.warning:
                self.store.add_warning(job_id, result.warning)
            if result.path:
                caption_file = result.path
                import json
                if result.segments:
                    json_captions = result.segments
                else:
                    json_captions = [
                        {"start": e[0], "end": e[1], "text": e[2]} for e in (result.entries or [])
                    ]
                self.store.update_job(
                    job_id,
                    captions_path=str(caption_file),
                    captions=json.dumps(json_captions),
                )
        elif caption_file is None and captions_path.exists():
            caption_file = captions_path

        self.store.set_state(job_id, "rendering", "rendering vertical mp4")
        self.renderer.render(
            input_path=raw_path,
            output_path=final_path,
            captions_path=caption_file,
            source_duration=source_duration,
            final_duration=float(job.get("final_duration") or self.cfg.final_duration),
            event_latency_offset=float(job.get("event_latency_offset") or self.cfg.event_latency_offset),
            trigger_detected_at_ms=int(job.get("trigger_detected_at") or 0),
            peak_chat_ts_ms=job.get("peak_chat_ts"),
        )
        self.store.set_state(
            job_id,
            "ready",
            "final render ready",
            final_path=str(final_path),
            artifact_available=1,
        )
        shutil.rmtree(job_dir, ignore_errors=True)

    def _fail(self, job_id: str, code: str, message: str) -> None:
        self.store.set_state(
            job_id,
            "failed",
            code,
            failure_code=code,
            error_message=message[:2000],
            artifact_available=0,
        )
