# StreamPulse product boundary

**Streamclone** (this public repository) is the self-hosted Twitch replica: live directory, HLS playback, IRC chat, 7TV/FFZ emotes, and the Windows desktop install. Local development runs the **core** compose stack on `http://localhost:8090` — metadata, video, chat, emote, frontend, and supporting infrastructure only.

**StreamPulse** is a separate hosted product (Chrome extension, analytics portal, hub API). It is not built, deployed, or documented from this repository. Backend source, operator runbooks, ingest evidence, and production promotion contracts live in **private** checkouts.

| Concern | Owner |
|---------|--------|
| Watch / playback / chat / emotes / install | **streamclone** (this repo) |
| Backend / BFF / ingest / shared packages | private **streampulse-backend** |
| Deploy, secrets, SSH, soak evidence | private **streampulse-ops** |
| Extension and portal UI + product docs | public **streamclone-pulse** (sibling checkout) |
| Clip Studio / ReplayForge | sibling **replayforge** (not advertised in Streamclone UI) |

Do not add hosted URLs, image tags, SSH paths, or operator topology to this public tree. Agent routing and docs here describe **core product scope only**.

**Step 7 complete (2026-07-09):** legacy analytics source trees, Clip Studio UI, `/studio` deeplinks, and `/v1/clipper` proxy removed from this repo. `make product-boundary-strict` fails on reintroduction of `cmd/analytics`, `internal/analytics` Go sources, `packages/analytics-console`, `/v1/analytics/`, ReplayForge UI strings, or `route @clipper`. Hosted backend and ops evidence live in private **streampulse-backend** and **streampulse-ops**.
