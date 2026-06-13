# Emote Pipeline Steering

## Current Implementation Review

The current project has a local 7TV-style emote system with 7TV and FFZ channel loading. It does not currently fetch or render native Twitch emote images from the IRC `emotes` tag. FFZ globals and provider SSE/TTL reconciliation are still future work.

Current provider/channel flow:

1. The frontend channel page resolves Twitch channel metadata with `getChannel(login)`.
2. The channel page shows a compact `Chat emotes` control with `7TV` and `FFZ` provider toggles inside the channel workspace. Keep this control and its progress states accessible when changing playback controls, density modes, or lower-panel layout.
3. The frontend calls `POST /v1/channels/{login}/emotes/ensure` with the Twitch user ID and selected providers.
4. `ensure` checks PostgreSQL for ready and pending channel/global emotes.
5. If selected providers are ready, the service rebuilds the Redis dictionary and returns `ready`.
6. If selected providers still need loading, the service starts a background provider seed and returns `processing`; the frontend polls every 5 seconds while processing.
7. If a seed is already running or assets are pending, `ensure` returns `processing` from local state before making remote provider refresh calls. Keep this fast path so VOD chat sync and 7TV polling do not wait on repeated 7TV API checks while assets are rendering.

Current provider seed flow:

1. For 7TV, the seeder requests `GET {SEVENTV_API_URL}/users/twitch/{twitch_id}`.
2. For FFZ, the seeder requests `GET {FFZ_API_URL}/room/id/{twitch_id}` with `GET {FFZ_API_URL}/room/{login}` as a fallback.
3. It upserts the channel, creates or finds a provider-selected emote set, and marks that set active for the channel.
4. It loops through provider emotes sequentially.
5. For each new provider emote, it downloads the selected source asset, records provider identity on the local emote row, stores source bytes in object storage, inserts a processing job, and adds the emote to the active set.
6. It rebuilds the Redis dictionary from active local assets only; pending assets appear after the worker marks them active and rebuilds affected dictionaries.

Current asset flow:

1. The worker claims `processing_jobs` rows with `FOR UPDATE SKIP LOCKED`.
2. It reads `{emote_id}/src` from object storage.
3. It uses libvips to render `1x`, `2x`, `3x`, and `4x` WebP files.
4. It writes `{emote_id}/{scale}.webp` objects, marks the emote active, and rebuilds dictionaries for affected channels.
5. On failure, it retries up to 3 attempts and avoids leaving partial rendered objects for failed uploads.

Current chat/rendering flow:

1. Twitch IRC messages are read server-side over anonymous IRC WebSocket.
2. The parser keeps message ID, channel, user, color, badges, timestamp, and text. It does not currently convert the Twitch IRC `emotes` tag into fragments.
3. The enricher loads `channel:emotes:{login}` from Redis into an in-memory Trie on first use.
4. Tokenization splits messages on spaces and matches whole words against the Trie.
5. Redis `emotes:delta:{login}` events trigger a debounced dictionary reload and atomic pointer swap.
6. The frontend renders `fragments[]`; emote fragments become `<img src={u} alt={c}>`.

## Gaps Against emote tokenizer roadmap

- Metadata and binary assets are not fully separated during seeding. The project now records provider identity, but the V1 provider loader still downloads selected channel emote assets immediately instead of using full lazy hydration.
- Asset loading is eager per channel, not lazy per observed emote/cache miss.
- The tokenizer is a whitespace whole-word Trie. It is fast for the current requirements, but it does not handle punctuation-adjacent emotes and does not use Aho-Corasick scanning.
- FFZ channel loading exists in the V1 provider loader, but FFZ globals and TTL/SSE style provider reconciliation are still future work.
- There is no 7TV EventAPI SSE listener. Live updates only come from local curator changes and local worker dictionary rebuilds.
- The current seed path can publish dictionary entries before rendered local `1x` assets are active, so future work should keep pending assets out of hot dictionaries unless a remote fallback URL is intentionally supported.
- Provider metadata is not modeled separately from local emote rows. External provider ID, provider set ID, source URL, version, and last-seen state are not first-class fields.

## Preferred Future Direction

Use a hybrid metadata index plus lazy local asset cache, while preserving the current local CDN/object-store model.

Metadata pipeline:

- Fetch provider metadata first and keep it separate from binary asset hydration.
- Add provider-aware metadata fields or tables before adding FFZ or more 7TV sync behavior. Useful fields include `provider`, `provider_emote_id`, `provider_set_id`, `provider_name`, `canonical_name`, `asset_url`, `animated`, `zero_width`, `owner_id`, `last_seen_at`, and `content_hash` when known.
- Fetch global and channel provider metadata concurrently when adding global 7TV/FFZ support.
- Keep all provider endpoints configurable.

Asset pipeline:

- Do not download an entire provider catalog.
- Do not eagerly download every channel asset if the goal is scale. Prefer lazy hydration when an emote is first observed in chat, requested by an API, or explicitly prewarmed by an admin task.
- On cache miss, enqueue hydration and return either a processing state or a deliberate remote fallback URL. If using remote fallback, make that behavior explicit in API contracts and UI states.
- Continue writing local WebP variants to object storage under the existing key layout unless a migration intentionally changes it.
- Bound provider download concurrency and add per-provider backoff/rate-limit handling.

Matching pipeline:

- Keep the current Trie for exact whitespace-delimited matching unless requirements change.
- If adopting the [emote tokenizer roadmap](../specs/emote-tokenizer-roadmap.md) target, update requirements/design first, then replace or augment the Trie with an Aho-Corasick automaton that can scan the whole message in linear time and handle punctuation boundaries deterministically.
- Rebuild matchers off the hot path and install them with the existing atomic pointer-swap pattern.
- Add tokenizer benchmarks before and after changing the matcher.

Live sync pipeline:

- For 7TV live updates, add a background SSE listener for `https://events.7tv.io/v3` only after provider metadata is modeled separately.
- Treat SSE as invalidation/delta input, not as the only source of truth. Reconcile against REST metadata when events are missed or connection state is uncertain.
- Use the existing Redis dictionary/delta fan-out after applying provider updates locally.

FFZ pipeline:

- Add FFZ as another metadata provider, not as special-case chat rendering code.
- Global metadata endpoint: `GET https://api.frankerfacez.com/v1/set/global`.
- Channel metadata endpoint: `GET https://api.frankerfacez.com/v1/room/id/{twitch_user_id}` or `GET https://api.frankerfacez.com/v1/room/{username}`.
- Store the returned URL map as provider asset candidates and hydrate local WebP variants lazily or through an explicit prewarm job.

Native Twitch emotes:

- Twitch IRC already sends native emote ranges in the `emotes` tag, but the current parser ignores them.
- If adding native Twitch emote rendering, parse ranges before custom dictionary tokenization so text offsets remain stable.
- Keep Twitch native emotes as a separate fragment source from local 7TV/FFZ custom emotes.

## Task Checklist For Emote Changes

- Read this file, [`.kiro/specs/emote-tokenizer-roadmap.md`](../specs/emote-tokenizer-roadmap.md), `.kiro/specs/twitch-7tv-clone/requirements.md`, and `.kiro/specs/twitch-7tv-clone/design.md` before changing emote behavior.
- Decide whether the task is metadata sync, asset hydration, tokenizer matching, live sync, frontend rendering, or schema work.
- Update requirements/design if behavior changes user-visible contracts or provider support.
- If the task touches the channel workspace, preserve the existing emote provider toggles, processing polling, and emote tab affordances.
- Keep metadata fetches, asset downloads, image processing, dictionary rebuilds, and chat tokenization as separate steps.
- Add or update tests for the narrow layer changed.
- For tokenizer work, include punctuation, spacing, repeated emotes, zero-width emotes, and non-matches.
- For provider sync work, include upstream failure, missing emote set, duplicate aliases, pending assets, and idempotent retry cases.
