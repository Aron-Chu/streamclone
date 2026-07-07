# Ops migration truth table (StreamPulse hosted)

**Read this first** when asking “did we move tags to the private repo?” or “why does Pulse show a release label in `/v1/extension/health`?”

The 2026-07 **ops boundary migration** moved **production control** into private **`streampulse-ops`**. It did **not** move release tags, application source, or customer data into a private-only tag system.

Related contracts:

- [hosted-production-ops.md](hosted-production-ops.md)
- [ops-migration-manifest.md](ops-migration-manifest.md)
- [production-artifact-contract.md](production-artifact-contract.md)
- [production-promotion-contract.md](production-promotion-contract.md)

---

## One-line summary

| Question | Answer |
|----------|--------|
| Did tags move to a private repo? | **No.** Git tags and GHCR **builds** stay on public **`Aron-Chu/streamclone`**. Private ops only pins **which tag runs** on the hosted production host. |
| What moved private? | Deploy scripts, production compose, **secrets**, **`IMAGE_TAG` pin**, smoke/rollback evidence. |
| Is a release label in Pulse health a security leak? | **No.** It is a **deploy label** echoed by `/v1/extension/health` — not a credential. |
| Can strangers deploy prod by knowing the tag? | **No.** Deploy needs private ops access + host secrets; pushing tags needs GitHub write access. |

---

## What migrated vs what did not

| Layer | Migrated to private `streampulse-ops`? | Still public `streamclone`? |
|-------|----------------------------------------|----------------------------|
| Go backend source | No | **Yes** |
| Git release tags | No | **Yes** |
| GHCR **source** images (`ghcr.io/aron-chu/streamclone/*`) | No | **Yes** |
| **`IMAGE_TAG` pin** (which release runs) | **Yes** | Contract docs only |
| Postgres / Redis / corpus **data** | No (stays on host) | N/A |
| Twitch / DB **secrets** | **Yes** | Never in git |
| Deploy / smoke / rollback scripts | **Yes** | Public API probes only |
| Deployment evidence manifests | **Yes** | Not in this repo |

---

## Safe public probes

```bash
curl -fsS https://api.streampulse.stream/v1/extension/health
bash scripts/hosted-launch-probes.sh
```

Operator SSH, internal ops routes, and VPS shell checks are documented in **private streampulse-ops** only.
