# Local Auth Steering

Local sign-in is optional and loopback-first. Watching should work without login.

## Flow

- User opens `http://localhost:8090`.
- Backend prepares one-time local claims.
- Twitch approval completes sign-in without passing tokens through URLs.
- Clipper credentials sync when Clip Studio is enabled.

## Rules

- Keep token import/dev auth loopback-only.
- Do not expose access or refresh tokens in URLs, logs, screenshots, or frontend-only flags.
- For public/tunnel origins, set `TWITCH_DEV_TOKEN_IMPORT_ENABLED=false`.
- Browser-visible setup-control and clipper tokens imply trusted visitors only.

## Checks

```sh
go test ./internal/chat/auth
make security-scan
```
