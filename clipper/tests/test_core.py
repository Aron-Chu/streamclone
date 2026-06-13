from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from liveclipper.db import Store
from liveclipper.irc import ChatMessage, VelocityDetector, parse_privmsg
from liveclipper.render import (
    build_audio_filter,
    build_filter,
    compute_trim_start,
    escape_filter_path,
    vod_moment_offset_in_source,
)
from liveclipper.streamlink import build_command
from liveclipper.templates import TemplateLoader, resolve_render_options
from liveclipper.templates import AudioEffects, IntroZoom, VideoEffects
from liveclipper.transcribe import (
    ass_time,
    build_style_line,
    detect_emote_tokens,
    format_karaoke_text,
    group_words_into_segments,
    offset_caption_segments,
    write_ass,
)
from liveclipper.vod import (
    build_ffmpeg_segment_argv,
    build_streamlink_json_argv,
    build_vod_page_url,
    compute_vod_segment_window,
    parse_streamlink_json,
)
from liveclipper.worker import JobWorker


class IRCTests(unittest.TestCase):
    def test_parse_privmsg_with_timestamp(self) -> None:
        line = "@display-name=Viewer;tmi-sent-ts=1000 :viewer!viewer@viewer.tmi.twitch.tv PRIVMSG #Streamer :hello"
        msg = parse_privmsg(line, 50)
        self.assertIsNotNone(msg)
        assert msg is not None
        self.assertEqual(msg.channel, "streamer")
        self.assertEqual(msg.user, "Viewer")
        self.assertEqual(msg.text, "hello")
        self.assertEqual(msg.timestamp_ms, 1000)

    def test_velocity_detector_spike_and_cooldown(self) -> None:
        detector = VelocityDetector(window_seconds=10, min_messages=3, multiplier=2.0, cooldown_seconds=60)
        self.assertIsNone(detector.observe(ChatMessage("chan", "", "a", 1000)))
        self.assertIsNone(detector.observe(ChatMessage("chan", "", "b", 2000)))
        spike = detector.observe(ChatMessage("chan", "", "c", 3000))
        self.assertIsNotNone(spike)
        self.assertIsNone(detector.observe(ChatMessage("chan", "", "d", 4000)))


class StoreTests(unittest.TestCase):
    def test_duplicate_suppression_by_broadcaster(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            store = Store(Path(td) / "clipper.sqlite")
            store.init()
            try:
                first = store.insert_job(
                    channel="chan",
                    broadcaster_id="123",
                    trigger_type="manual",
                    reason="one",
                    title="title",
                    requested_duration=60,
                    source_duration=60,
                    final_duration=30,
                    event_latency_offset=8,
                    trigger_detected_at=1000,
                    peak_chat_ts=None,
                    message_count=None,
                    duplicate_window_seconds=60,
                )
                second = store.insert_job(
                    channel="chan",
                    broadcaster_id="123",
                    trigger_type="manual",
                    reason="two",
                    title="title",
                    requested_duration=60,
                    source_duration=60,
                    final_duration=30,
                    event_latency_offset=8,
                    trigger_detected_at=2000,
                    peak_chat_ts=None,
                    message_count=None,
                    duplicate_window_seconds=60,
                )
                self.assertFalse(first.suppressed)
                self.assertTrue(second.suppressed)
                assert first.job_id is not None
                job = store.get_job(first.job_id)
                assert job is not None
                self.assertEqual(job["suppressed_count"], 1)
            finally:
                store.close()

    def test_moment_context_roundtrip(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            store = Store(Path(td) / "clipper.sqlite")
            store.init()
            try:
                ctx = {
                    "stream_id": "abc",
                    "chat_per_min": 42,
                    "pick_reason": "chat_spike",
                }
                result = store.insert_job(
                    channel="chan",
                    broadcaster_id="",
                    trigger_type="manual",
                    reason="analytics",
                    title="title",
                    requested_duration=60,
                    source_duration=60,
                    final_duration=30,
                    event_latency_offset=8,
                    trigger_detected_at=1000,
                    peak_chat_ts=None,
                    message_count=None,
                    duplicate_window_seconds=60,
                    moment_context=ctx,
                )
                assert result.job_id is not None
                job = store.get_job(result.job_id)
                assert job is not None
                self.assertEqual(job["moment_context"]["chat_per_min"], 42)
            finally:
                store.close()

    def test_duplicate_suppression_by_vod_offset(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            store = Store(Path(td) / "clipper.sqlite")
            store.init()
            try:
                ctx = {"vod_id": "12345", "vod_offset_seconds": 600}
                first = store.insert_job(
                    channel="chan",
                    broadcaster_id="",
                    trigger_type="vod_export",
                    reason="one",
                    title="title",
                    requested_duration=60,
                    source_duration=60,
                    final_duration=30,
                    event_latency_offset=8,
                    trigger_detected_at=1000,
                    peak_chat_ts=None,
                    message_count=None,
                    duplicate_window_seconds=60,
                    moment_context=ctx,
                )
                second = store.insert_job(
                    channel="chan",
                    broadcaster_id="",
                    trigger_type="vod_export",
                    reason="two",
                    title="title",
                    requested_duration=60,
                    source_duration=60,
                    final_duration=30,
                    event_latency_offset=8,
                    trigger_detected_at=2000,
                    peak_chat_ts=None,
                    message_count=None,
                    duplicate_window_seconds=60,
                    moment_context={"vod_id": "12345", "vod_offset_seconds": 630},
                )
                third = store.insert_job(
                    channel="chan",
                    broadcaster_id="",
                    trigger_type="vod_export",
                    reason="three",
                    title="title",
                    requested_duration=60,
                    source_duration=60,
                    final_duration=30,
                    event_latency_offset=8,
                    trigger_detected_at=3000,
                    peak_chat_ts=None,
                    message_count=None,
                    duplicate_window_seconds=60,
                    moment_context={"vod_id": "12345", "vod_offset_seconds": 900},
                )
                self.assertFalse(first.suppressed)
                self.assertTrue(second.suppressed)
                self.assertFalse(third.suppressed)
            finally:
                store.close()


class CommandTests(unittest.TestCase):
    def test_streamlink_bearer_header_redacted(self) -> None:
        cmd = build_command("https://clips.twitch.tv/test", Path("out.mp4"), "abcdef123456")
        joined = " ".join(cmd.argv)
        redacted = " ".join(cmd.redacted)
        self.assertIn("Authorization=Bearer abcdef123456", joined)
        self.assertNotIn("abcdef123456", redacted)
        self.assertIn("<redacted>", redacted)

    def test_trim_uses_chat_latency_offset(self) -> None:
        start = compute_trim_start(
            source_duration=60,
            final_duration=30,
            event_latency_offset=8,
            trigger_detected_at_ms=10_000,
            peak_chat_ts_ms=10_000,
        )
        self.assertAlmostEqual(start, 30.0)

    def test_trim_falls_back_to_tail(self) -> None:
        start = compute_trim_start(
            source_duration=60,
            final_duration=30,
            event_latency_offset=8,
            trigger_detected_at_ms=10_000,
            peak_chat_ts_ms=None,
        )
        self.assertEqual(start, 30)

    def test_trim_centers_on_vod_offset(self) -> None:
        moment_context = {
            "vod_id": "123",
            "vod_offset_seconds": 600,
            "vod_segment_start": 570.0,
            "source_kind": "vod",
        }
        start = compute_trim_start(
            source_duration=60,
            final_duration=30,
            event_latency_offset=8,
            trigger_detected_at_ms=10_000,
            peak_chat_ts_ms=None,
            moment_context=moment_context,
        )
        self.assertAlmostEqual(start, 19.5)

    def test_vod_moment_offset_in_source(self) -> None:
        offset = vod_moment_offset_in_source(
            {"vod_offset_seconds": 600, "vod_segment_start": 570},
            60,
        )
        self.assertAlmostEqual(offset or 0, 30.0)

    def test_subtitle_path_escaping(self) -> None:
        escaped = escape_filter_path(Path("C:/clipper data/captions,one.ass"))
        self.assertIn("C\\:", escaped)
        self.assertIn("captions\\,one.ass", escaped)


class CaptionTests(unittest.TestCase):
    def test_ass_time(self) -> None:
        self.assertEqual(ass_time(65.12), "0:01:05.12")

    def test_write_ass(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "captions.ass"
            write_ass(path, [(0, 1.2, "hello {world}")])
            data = path.read_text(encoding="utf-8")
            self.assertIn("[Events]", data)
            self.assertIn("hello \\{world\\}", data)

    def test_offset_caption_segments(self) -> None:
        segments = [
            {"start": 10.0, "end": 12.0, "text": "hello"},
            {"start": 5.0, "end": 8.0, "text": "drop me"},
        ]
        offset = offset_caption_segments(segments, trim_start=10.0, trim_end=25.0)
        self.assertEqual(len(offset), 1)
        self.assertAlmostEqual(offset[0]["start"], 0.0)
        self.assertAlmostEqual(offset[0]["end"], 2.0)

    def test_offset_word_level_captions(self) -> None:
        segments = [
            {
                "start": 10.0,
                "end": 13.0,
                "text": "one two",
                "words": [
                    {"text": "one", "start": 10.0, "end": 11.0},
                    {"text": "two", "start": 11.5, "end": 13.0},
                ],
            }
        ]
        offset = offset_caption_segments(segments, trim_start=10.0)
        self.assertEqual(len(offset[0]["words"]), 2)
        self.assertAlmostEqual(offset[0]["words"][0]["start"], 0.0)

    def test_karaoke_ass_output(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "karaoke.ass"
            segments = [
                {
                    "start": 0.0,
                    "end": 2.0,
                    "text": "hello world",
                    "words": [
                        {"text": "hello", "start": 0.0, "end": 1.0},
                        {"text": "world", "start": 1.0, "end": 2.0},
                    ],
                }
            ]
            write_ass(path, segments, style_preset="karaoke_pop", max_words_per_line=2)
            data = path.read_text(encoding="utf-8")
            self.assertIn("\\k", data)
            self.assertIn("hello", data)

    def test_format_karaoke_text(self) -> None:
        text = format_karaoke_text([
            {"text": "hi", "start": 0.0, "end": 0.5},
            {"text": "there", "start": 0.5, "end": 1.0},
        ])
        self.assertIn("\\k", text)
        self.assertIn("hi", text)

    def test_group_words_into_segments(self) -> None:
        words = [
            {"text": "a", "start": 0.0, "end": 0.3},
            {"text": "b", "start": 0.3, "end": 0.6},
            {"text": "c", "start": 0.6, "end": 0.9},
        ]
        grouped = group_words_into_segments(words, max_words_per_line=2)
        self.assertEqual(len(grouped), 2)
        self.assertEqual(grouped[0]["text"], "a b")

    def test_caption_size_and_position_in_ass(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "sized.ass"
            write_ass(
                path,
                [(0, 1.0, "hello")],
                caption_size="lg",
                caption_position="top",
            )
            data = path.read_text(encoding="utf-8")
            self.assertIn(",8,80,80,80,1", data)

    def test_build_style_line_scales_font(self) -> None:
        sm = build_style_line("default", caption_size="sm")
        lg = build_style_line("default", caption_size="lg")
        self.assertIn(",56,", sm)
        self.assertIn(",92,", lg)

    def test_detect_emote_tokens(self) -> None:
        found = detect_emote_tokens("OMEGALUL that was crazy", {"omegalul"})
        self.assertEqual(found, ["OMEGALUL"])

    def test_write_ass_returns_emote_hits(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "emotes.ass"
            hits = write_ass(
                path,
                [(0, 1.0, "KEKW moment")],
                emote_names={"kekw"},
            )
            self.assertEqual(len(hits), 1)
            self.assertEqual(hits[0]["name"], "KEKW")

    def test_write_ass_transform_and_effect(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "transform.ass"
            segments = [
                {
                    "start": 0.0,
                    "end": 2.0,
                    "text": "placed text",
                    "transform": {
                        "x": 0.5,
                        "y": 0.25,
                        "rotation": 15,
                        "scale": 1.2,
                    },
                    "effect": "pop",
                },
                {
                    "start": 2.0,
                    "end": 4.0,
                    "text": "shake line",
                    "transform": {"x": 0.1, "y": 0.9, "rotation": 0},
                    "effect": "shake",
                },
            ]
            write_ass(path, segments)
            data = path.read_text(encoding="utf-8")
            self.assertIn("\\pos(540,480)", data)
            self.assertIn("\\frz15.0", data)
            self.assertIn("\\fscx120\\fscy120", data)
            self.assertIn("\\fscx80\\fscy80\\t(0,150,\\fscx100\\fscy100)", data)
            self.assertIn("\\pos(108,1728)", data)
            self.assertIn("\\t(0,50,\\pos(116,1728))", data)
            self.assertIn("shake line", data)

    def test_write_ass_without_transform_unchanged(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "plain.ass"
            write_ass(path, [(0, 1.0, "plain caption")])
            data = path.read_text(encoding="utf-8")
            self.assertIn("plain caption", data)
            self.assertNotIn("\\pos(", data)


class TemplateTests(unittest.TestCase):
    def test_load_builtin_templates(self) -> None:
        loader = TemplateLoader()
        items = loader.list_public()
        self.assertGreaterEqual(len(items), 16)
        ids = {item["id"] for item in items}
        self.assertIn("tiktok_pop", ids)
        self.assertIn("gaming_punch", ids)
        self.assertIn("stacked_reaction", ids)
        self.assertIn("hype_zoom", ids)
        self.assertIn("slowmo_cinematic", ids)

    def test_resolve_render_options_from_template(self) -> None:
        loader = TemplateLoader()
        resolved = resolve_render_options(
            template_id="gaming_punch",
            format_preset=None,
            caption_preset=None,
            loader=loader,
        )
        self.assertEqual(resolved.format_preset, "tiktok")
        self.assertEqual(resolved.caption_preset, "gaming")
        self.assertIsNotNone(resolved.video_effects.intro_zoom)

    def test_build_filter_with_intro_zoom(self) -> None:
        effects = VideoEffects(intro_zoom=IntroZoom(duration=0.5, scale=1.08))
        graph = build_filter(None, "tiktok", effects)
        self.assertIn("zoompan", graph)
        self.assertIn("[v]", graph)

    def test_stacked_game_face_layout(self) -> None:
        graph = build_filter(None, "tiktok", layout="stacked_game_face", layout_split_ratio=0.35)
        self.assertIn("vstack", graph)
        self.assertIn("[v]", graph)

    def test_build_audio_filter_light_noise(self) -> None:
        chain = build_audio_filter(AudioEffects(noise_reduce="light"))
        self.assertIsNotNone(chain)
        assert chain is not None
        self.assertIn("afftdn", chain)


class VodTests(unittest.TestCase):
    def test_build_vod_page_url(self) -> None:
        self.assertEqual(build_vod_page_url("987654"), "https://www.twitch.tv/videos/987654")

    def test_compute_vod_segment_window(self) -> None:
        window = compute_vod_segment_window(600, 60)
        self.assertAlmostEqual(window.segment_start, 570.0)
        self.assertAlmostEqual(window.duration, 60.0)
        self.assertAlmostEqual(window.moment_offset, 30.0)

    def test_streamlink_json_argv_redacts_token(self) -> None:
        preview = build_streamlink_json_argv(
            "https://www.twitch.tv/videos/1",
            token="secret-token",
        )
        joined = " ".join(preview.argv)
        redacted = " ".join(preview.redacted)
        self.assertIn("Authorization=Bearer secret-token", joined)
        self.assertNotIn("secret-token", redacted)

    def test_build_ffmpeg_segment_argv(self) -> None:
        argv = build_ffmpeg_segment_argv(
            ffmpeg_bin="ffmpeg",
            stream_url="https://example.com/stream.m3u8",
            output_path=Path("out.mp4"),
            segment_start=120.5,
            duration=60.0,
        )
        self.assertEqual(argv[0], "ffmpeg")
        self.assertIn("-ss", argv)
        self.assertIn("120.500", argv)
        self.assertIn("60.000", argv)

    def test_parse_streamlink_json(self) -> None:
        url = parse_streamlink_json('{"type":"hls","url":"https://example.com/master.m3u8"}')
        self.assertEqual(url, "https://example.com/master.m3u8")


class WorkerRoutingTests(unittest.TestCase):
    def test_worker_routes_vod_jobs_before_create_clip(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            output_dir = Path(td) / "output"
            output_dir.mkdir(parents=True)
            cfg = load_test_config(output_dir)
            store = Store(output_dir / "clipper.sqlite")
            store.init()

            class FakeVodDownloader:
                def download_segment(self, **kwargs):
                    output_path = kwargs["output_path"]
                    output_path.write_bytes(b"vod")
                    from liveclipper.vod import VodSegmentWindow

                    return VodSegmentWindow(segment_start=570.0, duration=60.0, moment_offset=30.0)

            class FailingTwitch:
                def create_clip(self, **kwargs):
                    raise AssertionError("create_clip should not run for vod_export jobs")

            worker = JobWorker(
                cfg=cfg,
                store=store,
                twitch=FailingTwitch(),  # type: ignore[arg-type]
                downloader=object(),  # type: ignore[arg-type]
                vod_downloader=FakeVodDownloader(),  # type: ignore[arg-type]
                transcriber=object(),  # type: ignore[arg-type]
                renderer=object(),  # type: ignore[arg-type]
            )

            result = store.insert_job(
                channel="chan",
                broadcaster_id="",
                trigger_type="vod_export",
                reason="analytics",
                title="title",
                requested_duration=60,
                source_duration=60,
                final_duration=30,
                event_latency_offset=8,
                trigger_detected_at=1000,
                peak_chat_ts=None,
                message_count=None,
                duplicate_window_seconds=60,
                moment_context={"vod_id": "12345", "vod_offset_seconds": 600},
            )
            assert result.job_id is not None
            job = store.get_job(result.job_id)
            assert job is not None

            worker._finish_from_raw = lambda *args, **kwargs: None  # type: ignore[method-assign]
            worker._process(job)
            updated = store.get_job(result.job_id)
            assert updated is not None
            self.assertTrue(Path(updated["raw_path"]).exists())
            self.assertEqual(updated["moment_context"]["source_kind"], "vod")
            store.close()


def load_test_config(output_dir: Path):
    from liveclipper.config import Config

    return Config(
        host="127.0.0.1",
        port=8095,
        webhook_token="",
        twitch_client_id="",
        twitch_user_access_token="",
        twitch_api_url="https://api.twitch.tv/helix",
        twitch_irc_url="wss://irc-ws.chat.twitch.tv:443",
        data_dir=output_dir,
        db_path=output_dir / "clipper.sqlite",
        output_dir=output_dir,
        source_duration=60.0,
        final_duration=30.0,
        event_latency_offset=8.0,
        duplicate_window_seconds=60,
        cooldown_seconds=120,
        clip_poll_timeout_seconds=60,
        clip_poll_interval_seconds=1.0,
        chat_window_seconds=12,
        chat_min_messages=18,
        chat_spike_multiplier=2.5,
        asr_enabled=False,
        asr_required=False,
        whisper_model="small",
        whisper_compute_type="int8",
        streamlink_bin="streamlink",
        ffmpeg_encoder="libx264",
        ffmpeg_preset="veryfast",
        ffmpeg_bin="ffmpeg",
        final_retention_hours=48,
        cleanup_interval_seconds=3600,
        stale_job_seconds=120,
        auto_render=False,
    )


class ConfigTests(unittest.TestCase):
    def test_auto_render_defaults_false(self) -> None:
        import os

        from liveclipper.config import load_config

        saved = os.environ.pop("CLIPPER_AUTO_RENDER", None)
        try:
            cfg = load_config()
            self.assertFalse(cfg.auto_render)
        finally:
            if saved is not None:
                os.environ["CLIPPER_AUTO_RENDER"] = saved


if __name__ == "__main__":
    unittest.main()
