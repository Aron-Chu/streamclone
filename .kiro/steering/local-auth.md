# Local Auth Steering

## Current Local Flow

Streamclone supports localhost-only Twitch chat login for optional chat sending:

1. `Use local token` device-code flow on loopback (`http://localhost:8090`).
2. Optional `make twitch-local-auth` prepared claim for the same browser session.

Redirect OAuth (`/v1/auth/twitch/login` + `/v1/auth/twitch/callback`) was removed. Public deployments do not need Twitch redirect URLs in the Twitch Developer Console.

The local import path is enabled only when `TWITCH_DEV_TOKEN_IMPORT_ENABLED=true` and the request is loopback. The browser capability is reported by `GET /v1/me` as `canImportLocalToken`; do not replace that with a frontend-only flag.

The primary in-app local login path is the `Use local token` button. It should stay a one-click browser flow: first claim any prepared local token, and if none exists, start the backend-managed Twitch device-code flow in a modal.

`make twitch-local-auth` is an optional terminal helper. It syncs Twitch CLI app credentials into `.env`, makes sure the local proxy is running, runs the Twitch CLI device-code token flow, prepares a short-lived backend claim, and opens a localhost claim URL that sets the browser session cookie.

## Guardrails

- Keep Twitch access and refresh tokens out of query strings, fragments, logs, and frontend local storage.
- Keep token import and prepared-token claim endpoints loopback-only and gated by `TWITCH_DEV_TOKEN_IMPORT_ENABLED`.
- Keep manual access-token and refresh-token fields out of the primary login UI. The UI should prefer Twitch browser authentication over asking the user to touch tokens.
- Keep prepared claims short-lived and one-time-use. The claim URL may contain only an opaque local claim code, never OAuth token material.
- Keep the session cookie server-set and HttpOnly. The browser should not synthesize or persist auth cookies itself.
- Keep local browser, chat auth, frontend runtime config, and Caddy proxy aligned to `http://localhost:8090` unless intentionally testing a tunnel or deployment.
- If adding a new local auth helper, prefer using the existing backend validation flow so imported tokens still match `TWITCH_OAUTH_CLIENT_ID`.

## Task Checklist For Auth Changes

- Read this file plus `.kiro/steering/product.md` and `.kiro/steering/tech.md` before changing local auth behavior.
- Verify `GET /v1/me` still reports `canImportLocalToken` correctly for loopback and hides it for non-loopback.
- Verify token import rejects tokens whose Twitch `client_id` does not match the configured app.
- Verify `make twitch-local-auth` does not expose OAuth tokens in URLs.
- For backend auth changes, run `go test ./internal/chat/auth`.
- For frontend auth UI changes, run `npm run build` from `frontend/`.
