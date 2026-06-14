# Integration & Load Tests

## Prerequisites

- Docker (for test infrastructure containers)
- `libvips-tools` / `vips` CLI on the host running `TestUploadToActive` (matches emote worker)
- Go **1.25.11** (see root `go.mod`)

## Running Test Infrastructure

```bash
make integration-up
```

Or manually:

```bash
docker compose -f internal/integration/docker-compose.test.yml up -d
```

## Running Tests

Integration tests require `INTEGRATION=1`:

```bash
make integration-test
```

Load tests only (no containers):

```bash
go test ./internal/integration/ -run 'TestHighVelocity|TestTokenizer' -v -timeout 60s
```

## Stopping Infrastructure

```bash
make integration-down
```

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `INTEGRATION` | (unset) | Set to `1` to enable integration tests |
| `TEST_DATABASE_URL` | `postgres://app:test@localhost:15432/emotes?sslmode=disable` | PostgreSQL connection |
| `TEST_REDIS_ADDR` | `localhost:16379` | Redis address |
| `TEST_MINIO_ENDPOINT` | `localhost:19000` | MinIO endpoint |

## Test Coverage

### Integration Tests (require containers + `INTEGRATION=1`)

- `TestUploadToActive`: upload → worker (libvips) → active emote → MinIO scales → channel dictionary
- `TestSetChangeToDelta`: Redis dictionary add/remove → `emotes:delta:{login}` pub/sub

Schema is applied from real files under `migrations/` (`000002`–`000004`), not duplicated inline SQL.

### Load Tests (no external dependencies)

- `TestHighVelocityChatBoundedMemory`: 50k tokenizer messages, heap growth cap
- `TestHighVelocityChatBatchLatency`: multi-message batch flush latency from frame timestamps
- `TestTokenizerConcurrentSwapUnderLoad`: concurrent dictionary swap under read load
