# BearHost + Cloudflare Tunnel — Pulse extension API

**Production (2026-07-02 cutover):** `api.streampulse.stream` terminates at **streampulse-vps** (`23.173.152.156`). **BearHost** (`141.11.243.103`) remains a **rollback** host only until soak completes — no `cloudflared` connector there.

Public URL target:

```text
https://api.streampulse.stream
  → Cloudflare Tunnel (cloudflared on streampulse-vps)
  → http://127.0.0.1:8090
  → Caddy (Pulse API routes)
  → analytics + emote + metadata + corpus workers (single worker)
```

Rollback (operator only, not public DNS):

```text
BearHost localhost:8090  (stack up, no tunnel)
```

Reserved for later (do not configure yet):

| Hostname | Purpose |
|----------|---------|
| `app.streampulse.stream` | Web app / landing |
| `grafana.streampulse.stream` | Private ops (Cloudflare Access or Tailscale only) |

---

## VPS renewal note

**Current production:** full SoT on **streampulse-vps** (API + Postgres + Redis + single corpus worker + tunnel).

BearHost (`141.11.243.103`) is kept for **rollback** until post-cutover soak passes. Do not decommission BearHost until then.

Historical note — BearHost corpus scraping and Pulse live API **competed for the same 8 GB box**. That split topology is retired; corpus and API now colocate on streampulse-vps.

---

## 1. DNS (Cloudflare)

Add the domain **streampulse.stream** to Cloudflare. Tunnel routes create the `api` hostname — **no A record to BearHost IP** required when using Tunnel.

---

## 2. streampulse-vps — production Pulse API stack

On the VPS (`/opt/streamclone/app`, project `streamclone-production`):

```bash
# Beta key (never commit — create on VPS only)
sudo mkdir -p /etc/streamclone/secrets
sudo tee /etc/streamclone/secrets/pulse-beta.env <<'EOF'
PULSE_BETA_KEYS=replace-with-long-random-secret
EOF
sudo chmod 600 /etc/streamclone/secrets/pulse-beta.env
```

Sync repo, then switch to Pulse API mode:

```bash
bash scripts/bearhost-pulse-api.sh
```

Verify locally on the VPS:

```bash
curl -s http://127.0.0.1:8090/v1/extension/health
curl -s -H "X-Streamclone-Beta-Key: YOUR_KEY" \
  http://127.0.0.1:8090/v1/extension/pulse/channels/xqc | head -c 200
```

Services running: `postgres`, `redis`, `metadata`, `analytics` (Tier-0 IRC), `emote`, `minio`, `pulse-caddy` on **host port 8090**.

**Stopped in Pulse mode:** `analytics-workers`, `scraper`, `frontend`, `video`, `chat`, `mediamtx`.

---

## 3. Cloudflare Tunnel

1. Cloudflare dashboard → **Zero Trust** → **Networks** → **Tunnels** → select **`streampulse-bearhost`** (name retained from BearHost install).
2. **Rotate connector token** after cutover (token reuse from BearHost process args is a security risk). Issue a new token; install on **streampulse-vps only**; revoke old connectors.
3. Install connector on **streampulse-vps** (run the command Cloudflare shows, or `scripts/tmp/cf-tunnel-rotate.sh` with `CLOUDFLARE_API_TOKEN` set):

   ```bash
   sudo cloudflared service install <TOKEN_FROM_DASHBOARD>
   ```

4. Add **Public Hostname** (already configured if cutover complete):

   | Field | Value |
   |-------|--------|
   | Hostname | `api.streampulse.stream` |
   | Service | `http://localhost:8090` |

5. Confirm `cloudflared` is running:

   ```bash
   sudo systemctl status cloudflared
   ```

Example config: [`deploy/cloudflared/config.yml.example`](../../deploy/cloudflared/config.yml.example)

---

## 4. Test from your PC

```bash
curl -s https://api.streampulse.stream/v1/extension/health
curl -s -H "X-Streamclone-Beta-Key: YOUR_KEY" \
  https://api.streampulse.stream/v1/extension/pulse/channels/xqc | head -c 300
```

If health works but Pulse returns `401`, the beta key is missing or wrong.

---

## 5. Chrome extension

1. Extension options → **Backend URL:** `https://api.streampulse.stream`
2. **Beta key:** same value as `PULSE_BETA_KEYS` on the server.
3. Rebuild/reload the extension (`npm run build` in streamclone-pulse).

Manifest includes `https://api.streampulse.stream/*` host permission.

---

## 6. Env reference

| Variable | Pulse profile | Purpose |
|----------|---------------|---------|
| `PULSE_HOSTED_MODE` | `true` | Enables hosted guards |
| `PULSE_BETA_KEYS` | secret file | Comma-separated beta keys (required for gating) |
| `PULSE_MAX_ACTIVE_CHANNELS` | `10` | IRC tracking cap |
| `PULSE_MAX_BACKFILLS` | `1` | Concurrent extension backfills |
| `PULSE_MAX_CHANNELS_PER_PRINCIPAL` | `10` | Per-principal channel cap (parsed; enforcement pending) |
| `PULSE_WATCH_RATE_PER_MIN` | `6` | Watch requests per minute per principal (parsed; enforcement pending) |
| `PULSE_BACKFILL_RATE_PER_HOUR` | `5` | Backfill jobs per hour per principal (parsed; enforcement pending) |
| `SEVENTV_EVENTAPI_ENABLED` | `true` | Live 7TV emote deltas via EventAPI |
| `STREAMCLONE_VERSION` | from `VERSION` | Shown in `/v1/extension/health` |
| `TIER0_ENABLED` | `true` | Live viewer samples + roster |
| `CORPUS_WORKERS_ENABLED` | `0` | No corpus plane on analytics-workers |

---

## 7. Switch back to corpus mode

```bash
bash scripts/bearhost-corpus-only.sh
```

---

## Remote ops from Windows

```powershell
# WSL + BearHost SSH key
wsl bash -lc 'export BEARHOST_SSH_KEY=$HOME/.ssh/id_ed25519_bearhost_streamclone; cd /mnt/c/Users/Aron/twitch-7tv-clone && bash scripts/bearhost-pulse-api-remote.sh'
```
