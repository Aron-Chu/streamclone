# Streamclone data MCP

Read-only Postgres and Redis inspection for local compose (`5432`, `6379`).

## Setup

```sh
make codegraph-install
```

Adds `psycopg` and `redis` to `.codegraph/.venv`.

## Tools

- `data_status` — port reachability
- `postgres_query` — SELECT-only SQL
- `emote_jobs` — processing_jobs snapshot
- `redis_get` — fetch a key
- `redis_channel_emotes` — `channel:emotes:{login}` preview
