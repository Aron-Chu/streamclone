# Integration Tests

Requires Docker. `TestUploadToActive` also needs host `vips`.

```sh
make integration-up
make integration-test
make integration-down
```

Manual load-only checks:

```sh
go test ./internal/integration/ -run 'TestHighVelocity|TestTokenizer' -v -timeout 60s
```

Defaults:

| Variable | Default |
|----------|---------|
| `TEST_DATABASE_URL` | `postgres://app:test@localhost:15432/emotes?sslmode=disable` |
| `TEST_REDIS_ADDR` | `localhost:16379` |
| `TEST_MINIO_ENDPOINT` | `localhost:19000` |
