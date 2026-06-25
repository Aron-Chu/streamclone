# Chat benchmark planning scaffold (local-only)

`chat_benchmark_plan.py` supports **Batch P1-MAX Lane D** and future `BENCH-STORAGE-001` planning.

## Safety defaults

- **`--no-network` is the default** (implicit when `--allow-live` is omitted).
- Reads fixture markdown metadata only; **no Twitch, GQL, IRC, gold, or scraper calls**.
- `--allow-live` is reserved for a future explicitly approved batch; it is **not implemented** and exits with code 2.

## Usage

```powershell
cd c:\Users\Aron\twitch-7tv-clone

python scripts/bench/chat_benchmark_plan.py --help

python scripts/bench/chat_benchmark_plan.py `
  --fixture scripts/bench/fixtures/sample-light.md `
  --no-network
```

Fixture docs may also live in **streamclone-pulse** — pass an absolute or relative path:

```powershell
python scripts/bench/chat_benchmark_plan.py `
  --fixture ..\streamclone-pulse\docs\pulse-extension\fixtures\chat-benchmark-small.md `
  --no-network
```

## Fixture format

Markdown table with keys such as `fixture_class`, `message_count`, `avg_message_bytes`.

## Does not complete

This scaffold does **not** complete runtime benchmark tasks (`BENCH-002`–`005`, `LOAD-CHAT-001`, `LOAD-GOLD-001`).
