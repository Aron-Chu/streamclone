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

**Step 7 preflight (2026-07-08):** strict boundary **91 hits** (unchanged after Batch 9 setup-control pulse hook removal; was 99 after Batch 8). **Autonomous cleanup complete** — public install/scripts/docs surfaces are clean; remaining hits are **`internal/config/config.go` + tests (89)** and **optional migrations (2, paused)**. Legacy analytics trees remain on disk (excluded from strict glob) until ops digest confirms backend image on prod. Mirror verification → `docs/split/mirror-verification.md`; boundary grep → `make product-boundary-preflight`. **Blocked on:** private **streampulse-ops** hosted cutover digest + final deletion PR — do **not** delete legacy trees until digest evidence is recorded.
