from __future__ import annotations

import asyncio
import random
import threading
import time
from collections import defaultdict, deque
from dataclasses import dataclass
from typing import Callable


@dataclass(frozen=True)
class ChatMessage:
    channel: str
    user: str
    text: str
    timestamp_ms: int


@dataclass(frozen=True)
class Spike:
    channel: str
    peak_chat_ts: int
    message_count: int
    reason: str


def parse_privmsg(line: str, fallback_ts_ms: int) -> ChatMessage | None:
    rest = line
    tags: dict[str, str] = {}
    if rest.startswith("@"):
        raw, _, rest = rest.partition(" ")
        for part in raw[1:].split(";"):
            key, sep, value = part.partition("=")
            if sep:
                tags[key] = value
    if rest.startswith(":"):
        _, _, rest = rest.partition(" ")
    cmd, _, rest = rest.partition(" ")
    if cmd != "PRIVMSG":
        return None
    channel_raw, sep, text = rest.partition(" :")
    if not sep:
        return None
    channel = channel_raw.strip().lstrip("#").lower()
    if not channel:
        return None
    raw_ts = tags.get("tmi-sent-ts", "")
    try:
        ts = int(raw_ts) if raw_ts else fallback_ts_ms
    except ValueError:
        ts = fallback_ts_ms
    user = tags.get("display-name") or tags.get("login") or ""
    return ChatMessage(channel=channel, user=user, text=text, timestamp_ms=ts)


class VelocityDetector:
    def __init__(
        self,
        *,
        window_seconds: int,
        min_messages: int,
        multiplier: float,
        cooldown_seconds: int,
    ):
        self.window_ms = window_seconds * 1000
        self.min_messages = min_messages
        self.multiplier = multiplier
        self.cooldown_ms = cooldown_seconds * 1000
        self.messages: dict[str, deque[int]] = defaultdict(deque)
        self.baseline: dict[str, float] = defaultdict(lambda: max(1.0, min_messages / max(multiplier, 1.0)))
        self.last_trigger: dict[str, int] = {}

    def observe(self, message: ChatMessage) -> Spike | None:
        q = self.messages[message.channel]
        q.append(message.timestamp_ms)
        cutoff = message.timestamp_ms - self.window_ms
        while q and q[0] < cutoff:
            q.popleft()
        count = len(q)
        baseline = self.baseline[message.channel]
        last = self.last_trigger.get(message.channel)
        if last is not None and message.timestamp_ms - last < self.cooldown_ms:
            self._update_baseline(message.channel, count)
            return None
        threshold = max(self.min_messages, int(round(baseline * self.multiplier)))
        if count >= threshold:
            self.last_trigger[message.channel] = message.timestamp_ms
            reason = f"chat spike: {count} messages/{self.window_ms // 1000}s"
            self._update_baseline(message.channel, count)
            return Spike(message.channel, message.timestamp_ms, count, reason)
        self._update_baseline(message.channel, count)
        return None

    def _update_baseline(self, channel: str, count: int) -> None:
        current = self.baseline[channel]
        self.baseline[channel] = max(1.0, current * 0.98 + count * 0.02)


class IRCMonitor:
    def __init__(
        self,
        *,
        irc_url: str,
        detector: VelocityDetector,
        trigger: Callable[[Spike], None],
        note_error: Callable[[str, str], None],
    ):
        self.irc_url = irc_url
        self.detector = detector
        self.trigger = trigger
        self.note_error = note_error
        self.channels: set[str] = set()
        self._lock = threading.RLock()
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None
        self._loop: asyncio.AbstractEventLoop | None = None

    def start(self) -> None:
        if self._thread and self._thread.is_alive():
            return
        self._thread = threading.Thread(target=self._run, name="clipper-irc", daemon=True)
        self._thread.start()

    def stop(self) -> None:
        self._stop.set()
        if self._loop:
            self._loop.call_soon_threadsafe(lambda: None)
        if self._thread:
            self._thread.join(timeout=3)

    def watch(self, channel: str) -> None:
        with self._lock:
            self.channels.add(channel.lower())

    def unwatch(self, channel: str) -> None:
        with self._lock:
            self.channels.discard(channel.lower())

    def _run(self) -> None:
        self._loop = asyncio.new_event_loop()
        asyncio.set_event_loop(self._loop)
        self._loop.run_until_complete(self._main())

    async def _main(self) -> None:
        backoff = 1.0
        while not self._stop.is_set():
            try:
                await self._connect_once()
                backoff = 1.0
            except Exception as exc:
                for channel in self._snapshot_channels():
                    self.note_error(channel, str(exc))
                await asyncio.sleep(backoff + random.random())
                backoff = min(backoff * 2, 30)

    async def _connect_once(self) -> None:
        try:
            import websockets
        except ImportError as exc:
            raise RuntimeError("websockets is not installed") from exc
        async with websockets.connect(self.irc_url, ping_interval=None) as ws:
            nick = "justinfan" + str(random.randint(10000, 99999))
            await ws.send("PASS SCHMOOPIIE\r\n")
            await ws.send("NICK " + nick + "\r\n")
            await ws.send("CAP REQ :twitch.tv/tags twitch.tv/commands\r\n")
            joined: set[str] = set()
            while not self._stop.is_set():
                for channel in self._snapshot_channels():
                    if channel not in joined:
                        await ws.send("JOIN #" + channel + "\r\n")
                        joined.add(channel)
                for channel in list(joined):
                    if channel not in self._snapshot_channels():
                        await ws.send("PART #" + channel + "\r\n")
                        joined.remove(channel)
                try:
                    data = await asyncio.wait_for(ws.recv(), timeout=5)
                except asyncio.TimeoutError:
                    continue
                for line in str(data).rstrip("\r\n").split("\r\n"):
                    await self._handle_line(ws, line)

    async def _handle_line(self, ws: object, line: str) -> None:
        if line.startswith("PING"):
            await ws.send("PONG :tmi.twitch.tv\r\n")
            return
        msg = parse_privmsg(line, int(time.time() * 1000))
        if not msg:
            return
        spike = self.detector.observe(msg)
        if spike:
            self.trigger(spike)

    def _snapshot_channels(self) -> set[str]:
        with self._lock:
            return set(self.channels)
