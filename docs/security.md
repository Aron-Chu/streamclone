# Security and legal

## Legal / Terms of Service notice

This project accesses third-party internal endpoints (Twitch internal GraphQL, Usher, anonymous IRC, 7TV v3 API) that are not part of any public developer programme. It is provided solely for **educational and personal self-hosting purposes**. It is **not affiliated with, endorsed by, or sponsored by Twitch Interactive, Inc. or 7TV**.

Operating this software may violate the Terms of Service of Twitch, 7TV, and other upstream platforms, as well as applicable laws in your jurisdiction. **The operator is solely responsible for ensuring compliance with all relevant terms, licenses, and laws.** The authors accept no liability for misuse.

This project reads from third-party internal endpoints and restreams third-party content. Using it **may violate those services' Terms of Service** and applicable laws. The operator is **solely responsible** for all compliance obligations.

## Viewer-facing access model

Viewer-facing read endpoints (directory, stream start, anonymous chat listening) and the emote asset CDN remain available without a first-party account. Sending real chat messages uses Twitch OAuth, stores Twitch tokens server-side in Redis, and sends through an authenticated IRC connection for the logged-in Twitch user. The app does not maintain its own username/password account system.

Viewer-facing APIs are read-only and unauthenticated by design.

## Curator / Admin API

The Emote Service exposes a curator API for managing the emote database:

- Uploading emotes (multipart; validated, hashed, and processed asynchronously)
- Creating and editing emote sets, adding/removing emotes with optional per-set aliases
- Assigning an active emote set to a channel
- Loading channel emotes from selected 7TV and FFZ providers (`POST /v1/channels/{login}/emotes/ensure`)
- Seeding emote data and assets from the public 7TV v3 API (`POST /v1/seed/twitch/{twitch_id}`)

All write/admin endpoints require an `Authorization: Bearer <CURATOR_API_TOKEN>` header. **This token must be set to a strong, unpredictable value before exposing the service outside localhost.** Deploying with the default `change-me` value is a security defect.

## Deployment hardening

For any public-facing deployment, operators should:

- Place the entire stack behind a TLS-terminating reverse proxy (nginx, Caddy, Traefik, etc.).
- Apply rate limiting at the proxy layer for stream start, search, and WebSocket connect endpoints.
- Restrict the MinIO console port (9001) to trusted networks.
- Rotate `CURATOR_API_TOKEN`, `AUTH_COOKIE_SECRET`, Twitch OAuth credentials, and object-store credentials from the defaults before deployment.

## Application security

The services validate all client-supplied inputs (channel names, pagination, upload type/size), use parameterised SQL queries, pass subprocess arguments as argv slices (never shell strings), and render chat text as plain text nodes — not innerHTML — to prevent injection.
