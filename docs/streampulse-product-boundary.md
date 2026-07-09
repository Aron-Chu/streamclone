# StreamPulse product boundary

**Streamclone** (this public repository) is the self-hosted Twitch replica: live directory, HLS playback, IRC chat, 7TV/FFZ emotes, and the Windows desktop install. Local development runs the **core** compose stack on `http://localhost:8090` — metadata, video, chat, emote, frontend, and supporting infrastructure only.

**StreamPulse** is a separate hosted product (Chrome extension, analytics portal, hub API). It is not built, deployed, or documented from this repository. Backend source, operator runbooks, ingest evidence, and production promotion contracts live in **private** checkouts.

| Concern | Owner |
|---------|--------|
| Watch / playback / chat / emotes / install | **streamclone** (this repo) |
| Backend / BFF / ingest / shared packages | private **streampulse-backend** |
| Deploy, secrets, SSH, soak evidence | private **streampulse-ops** |
| Extension and portal UI + product docs | public **streamclone-pulse** (sibling checkout) |

Do not add hosted URLs, image tags, SSH paths, or operator topology to this public tree. Legacy analytics code may remain temporarily during the boundary split; agent routing and docs here describe **core product scope only**.

**Step 7 preflight (2026-07-08):** strict boundary **110 hits** (was 119 after Batch 6 ops/tooling trim; 152 pre-Batch 4). Remaining hits: legacy analytics trees, `internal/config/config.go` PULSE fields, frontend deep links (needs-decision), allowlists and deferred script surfaces. Mirror verification → `docs/split/mirror-verification.md`; boundary grep → `make product-boundary-preflight`. Do **not** delete legacy analytics trees until private **streampulse-backend** image/digest is confirmed on production via **streampulse-ops**.
