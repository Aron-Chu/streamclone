# Streamclone Live Clipper

Standalone local app for turning Twitch chat spikes or Streamer.bot webhooks into vertical MP4 clips.

## Quick Start

```sh
cd clipper
python3 -m venv .venv
. .venv/bin/activate
python -m pip install -r requirements.txt
CLIPPER_TWITCH_CLIENT_ID=... \
CLIPPER_TWITCH_USER_ACCESS_TOKEN=... \
python -m liveclipper
```

Open `http://127.0.0.1:8095`.

## Useful Commands

From the repo root:

```sh
make clipper-test CLIPPER_PYTHON=clipper/.venv/bin/python
make clipper-run CLIPPER_PYTHON=clipper/.venv/bin/python
```

`make clipper-run` expects runtime dependencies to already be installed in the interpreter you pass with `CLIPPER_PYTHON`.

## Required Tools

- Python 3.12+
- FFmpeg on `PATH`, or the bundled `imageio-ffmpeg` fallback
- Streamlink on `PATH`, in the clipper venv, or set with `CLIPPER_STREAMLINK_BIN`
- Twitch user access token with clip creation permissions

## Local Defaults

- API/dashboard: `http://127.0.0.1:8095`
- SQLite database: `clipper-data/clipper.sqlite`
- Output directory: `clipper-data/output`
- One render job at a time
- Final MP4 retention: 48 hours

See `.kiro/steering/clipper.md` and `.kiro/specs/live-clipper` before changing behavior.
