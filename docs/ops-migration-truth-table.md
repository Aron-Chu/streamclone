# Ops migration truth table (stub)

Historical note on the 2026-07 **ops boundary migration**: production **control** moved into private **streampulse-ops**. Application source, git release tags, and GHCR **source** image builds remain on public **streamclone**.

| Question | Answer |
|----------|--------|
| Did tags move to a private repo? | **No.** Git tags and GHCR builds stay on public **Aron-Chu/streamclone**. Private ops only pins **which tag runs** on the hosted production host. |
| What moved private? | Deploy scripts, production compose, **secrets**, **image pin**, smoke/rollback evidence. |
| Is a deploy label in hosted health JSON a credential leak? | **No.** It is a release/deploy label echoed by the hosted health endpoint — not a secret. |
| Can strangers deploy prod by knowing the tag? | **No.** Deploy needs private ops access + host secrets; pushing tags needs GitHub write access. |

## What migrated vs what did not

| Layer | Migrated to private streampulse-ops? | Still public streamclone? |
|-------|--------------------------------------|---------------------------|
| Go backend source | No | **Yes** (legacy trees during split) |
| Git release tags | No | **Yes** |
| GHCR **source** images | No | **Yes** |
| **Image pin** (which release runs) | **Yes** | Contract docs only |
| Postgres / Redis / corpus **data** | No (stays on host) | N/A |
| Twitch / DB **secrets** | **Yes** | Never in git |
| Deploy / smoke / rollback scripts | **Yes** | Public contract stubs only |
| Deployment evidence manifests | **Yes** | Not in this repo |

## Public contract stubs

- [streampulse-product-boundary.md](streampulse-product-boundary.md)
- [hosted-ops-stub.md](hosted-ops-stub.md)
- [ops-migration-manifest.md](ops-migration-manifest.md) (historical)

Hosted health probes and operator SSH are documented in **private streampulse-ops** only.
