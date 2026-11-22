# Design note: one shared dictionary for global emotes

**Status:** Proposed. Not built.
**Date:** 2026-11-22

---

## Problem

Every channel's dictionary, `channel:emotes:<login>`, holds every global
emote. `store.GetChannelEmotes` adds every `is_global` emote row to the
channel's own set, and `dict.Rebuild` writes the result into the channel's
hash. So:

- Redis memory grows with *dictionaries × global emotes*.
- A change to a provider's global list reaches a channel only when that
  channel's dictionary is next rebuilt.
- Nothing clears `is_global` on a provider emote row once it is set. An emote
  that a provider drops from its global list stays in every dictionary.

## What is already in place

The change that adds this note also:

- seeds every provider into one canonical set per channel, so ensure and the
  roster preloader agree on what a channel's dictionary holds;
- fetches each provider's global list once per preload cycle
  (`seeder.GlobalCatalog`);
- resolves same-name emotes in Go with a fixed order (`store.channelEmoteBeats`):
  the channel's own emote beats a global one, then custom, 7TV, BTTV, FFZ and
  Twitch, then the lowest emote id.

The design below keeps that order. It only changes where the globals are
stored.

## Measured cost

Measured locally on 2026-11-22. The public global lists of 7TV, BTTV and FFZ
were fetched, turned into dictionary entries the way `dict.Rebuild` writes
them, and written to a local Redis 7.4 with default settings. `MEMORY USAGE`
gave the size. The entry values are longer than 64 bytes, so these hashes
always use Redis's hashtable encoding.

| Global emotes in each dictionary | Count | Redis memory per dictionary |
|---|---|---|
| 7TV, BTTV, FFZ (measured) | 125 (45 + 65 + 15) | 25.4 KB (about 204 B each) |
| Plus 150 stand-in Twitch globals | 275 | 66.6 KB (Twitch entries are about 274 B each) |

Listing Twitch's global emotes needs an app token, so that list was not
fetched. The stand-ins use Twitch-style ids and names. The per-dictionary
cost hardly depends on the channel's own emotes: 25 to 32 KB for 0 to 1,000
channel emotes.

| Dictionaries | 7TV + BTTV + FFZ globals | With 150 Twitch globals |
|---|---|---|
| 1,000 | 25 MB | 67 MB |
| 10,000 | 254 MB | 666 MB |
| 20,000 | 509 MB | 1.33 GB |

With shared globals, all of this becomes one hash of 25 to 67 KB.

Before building, check three numbers on the real deployment:

1. How many global rows the dictionaries really carry:
   `SELECT provider, count(*) FROM emotes WHERE is_global AND status = 1 GROUP BY provider`.
   Stale rows (see above) make this larger than the providers' lists.
2. How many `channel:emotes:*` keys exist at the daily peak.
3. How many global emotes Twitch lists.

## Design

### Keys

- `emotes:global` holds one entry per global emote name, in the same entry
  format as channel dictionaries. Same-name globals are already resolved with
  the order above, so this hash has exactly one winner per name. The name does
  not match `channel:emotes:*`, so the TTL backfill scan skips it.
- `channel:emotes:<login>` holds only the channel's own emotes: the items of
  the channel's active set whose emote row is not global, plus global rows
  that the set lists under an alias (the alias is channel-specific).
- `emotes:delta:global` carries a `reload` message whenever `emotes:global`
  is rebuilt. Channel deltas stay on `emotes:delta:<login>`.

Both hashes are rebuilt the way `dict.Rebuild` works today: a staging key,
then `RENAME`, inside one `MULTI`/`EXEC`, with a TTL.

### Writer

The emote service builds `emotes:global` from PostgreSQL (all global rows with
status 1, resolved by name), not from the provider responses, so it always
matches what the merged dictionaries hold today. It rebuilds it:

- at the end of each preload cycle that fetched global lists;
- after any seed that imported global emotes, for installs that run without
  the preloader;
- at startup when the key is missing.

### Readers

Every reader looks a name up in the channel hash first and in the global hash
second. That is the same "channel beats global" rule as above, so readers that
merge give the same answer as today's merged dictionary.

| Reader | Change |
|---|---|
| Chat enricher (`internal/chat/enrich`) | Keep one global trie per process, loaded from `emotes:global` and reloaded on `emotes:delta:global`. Look up the channel trie first, then the global trie. A global change then needs no per-channel rebuild. |
| Emote list API (`GET /v1/channels/{login}/emotes`) | None. It reads PostgreSQL through `GetChannelEmotes`, which already returns the merged, resolved list. |
| Readers outside this repository | Any service that reads `channel:emotes:<login>` and reloads on `emotes:delta:<login>` must also load `emotes:global`, subscribe to `emotes:delta:global` and merge with the channel winning, before the split step below. |

### An empty channel dictionary

Today a missing or empty hash means "not loaded yet", and the chat enricher
asks for an ensure. With channel-only hashes, a channel with no emotes of its
own would look the same forever. So every channel hash also gets a marker
field that no emote can be named, for example the field `"\x00v"` with value
`"2"`. Readers skip fields that start with `\x00`. Names such as `:)` and
`<3` are real Twitch emotes, so punctuation prefixes are not safe.

## Rollout behind a flag

One setting on the emote service, for example
`EMOTE_GLOBAL_DICTIONARY=off|dual|split`, plus reader support that ships
before it is used.

1. **Readers first.** Ship readers that merge `channel:emotes:<login>` with
   `emotes:global` when the global key exists, and read the merged dictionary
   alone when it does not. While dictionaries are still merged, this changes
   nothing a viewer sees.
2. **`dual`.** The writer keeps writing merged channel dictionaries and also
   writes `emotes:global`. Compare the two for a sample of channels (a metric
   or a one-off script): every global in a merged dictionary must be in the
   global hash, and no channel winner may differ.
3. **`split`.** The writer drops globals from channel dictionaries. The memory
   saving arrives as dictionaries are rebuilt, within one TTL (24 hours by
   default) or one preload cycle. Switch only after every reader, including
   readers outside this repository, has shipped step 1.

### Rollback

- From `split` to `dual`: set the flag back. Readers keep merging, so viewers
  see no gap. Channel dictionaries regain their globals as they are rebuilt.
  Rebuild them all at once only if a reader without merge support has to come
  back.
- From `dual` to `off`: set the flag back and delete `emotes:global`. Readers
  fall back to the merged dictionaries.

## Not covered here

- Picking up the 7TV alias in `Data.Name`, and refreshing every provider with
  removals, are separate changes.
- Clearing `is_global` on rows that providers no longer list. The global hash
  makes stale globals cheaper but does not remove them.
