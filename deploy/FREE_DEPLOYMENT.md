# Free HTTPS Deployment

This app needs a VM, not just static hosting: it runs the React frontend plus Go services, Redis, PostgreSQL, MinIO, MediaMTX, Streamlink, and FFmpeg.

The cheapest practical path is:

1. A free/always-free Linux VM.
2. A free DNS name pointed at that VM.
3. Caddy terminating HTTPS and routing one public origin to the internal Docker services.

## Recommended Free-ish Stack

- VM: Oracle Cloud Always Free Ampere A1, Ubuntu, 2-4 OCPU, 8-24 GB RAM.
- DNS: DuckDNS free subdomain, or any real domain you already own.
- HTTPS/proxy: Caddy from `deploy/Caddyfile`.
- Runtime: Docker Compose with `deploy/docker-compose.yml` plus `deploy/docker-compose.prod.yml`.

Cloud capacity and free-tier rules can change. Keep the VM inside the provider's Always Free limits.

## Public URLs

Replace `yourname.duckdns.org` with your actual host:

- App: `https://yourname.duckdns.org`
- Optional chat login: localhost device-code only (no public OAuth redirect required)
- WebSocket: `wss://yourname.duckdns.org/v1/ws`
- HLS: `https://yourname.duckdns.org/live/{channel}/index.m3u8`

## 1. Create The VM

Create an Ubuntu VM with at least:

- 2 OCPU / 8 GB RAM for testing.
- 4 OCPU / 16-24 GB RAM if you want smoother stream workers.
- 80-150 GB boot volume if you plan to keep images/assets.

Open only these inbound ports in the cloud firewall/security list:

- `22/tcp` from your IP for SSH.
- `80/tcp` from the internet for Let's Encrypt HTTP validation.
- `443/tcp` from the internet for the app.

Do not expose Redis, Postgres, MinIO console, RTMP, or service ports publicly.

## 2. Point DNS At The VM

For DuckDNS:

1. Create a subdomain, for example `yourname.duckdns.org`.
2. Point it at the VM public IPv4.
3. Confirm it resolves before starting Caddy.

```sh
nslookup yourname.duckdns.org
```

## 3. Install Docker

On the VM:

```sh
sudo apt update
sudo apt install -y ca-certificates curl git
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER
newgrp docker
docker compose version
```

## 4. Copy The Repo

```sh
git clone <your-repo-url> streamclone
cd streamclone
cp .env.example .env
```

If you do not use GitHub yet, zip/copy the folder to the VM and run the same commands from the project root.

## 5. Configure `.env`

Minimum production-ish values:

```env
APP_DOMAIN=yourname.duckdns.org
ACME_EMAIL=you@example.com

FRONTEND_ORIGIN=https://yourname.duckdns.org
# TWITCH_OAUTH_REDIRECT_URL is optional legacy; redirect OAuth login was removed.
AUTH_COOKIE_SAMESITE=lax

HLS_PUBLIC_BASE=https://yourname.duckdns.org
CDN_PUBLIC_BASE=https://yourname.duckdns.org

TWITCH_OAUTH_CLIENT_ID=your_twitch_app_client_id
TWITCH_OAUTH_CLIENT_SECRET=your_twitch_app_client_secret
AUTH_COOKIE_SECRET=replace-with-a-long-random-value
CURATOR_API_TOKEN=replace-with-a-long-random-value

MINIO_ROOT_USER=replace-minio-user
MINIO_ROOT_PASSWORD=replace-minio-password
S3_ACCESS_KEY=replace-minio-user
S3_SECRET_KEY=replace-minio-password
```

For random secrets:

```sh
openssl rand -hex 32
```

## 6. Twitch app credentials

Create a Twitch Developer application and set `TWITCH_OAUTH_CLIENT_ID` / `TWITCH_OAUTH_CLIENT_SECRET` in `.env` for Helix and optional localhost chat login. You do **not** need to register OAuth redirect URLs for public deployment — anonymous viewing works without login.

## 7. Start The Stack

```sh
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml up -d --build
```

Watch startup:

```sh
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml ps
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml logs -f caddy chat frontend
```

## 8. Smoke Test

```sh
curl -I https://yourname.duckdns.org
curl https://yourname.duckdns.org/v1/auth/debug
curl https://yourname.duckdns.org/v1/auth/debug
```

Expected:

- App returns `200`.
- Auth debug shows `ready: true` when Twitch client id/secret are configured.

Then open:

```text
https://yourname.duckdns.org
```

Browse and watch channels without logging in.

## Notes

- Free static hosts alone will not run this stack because stream workers need FFmpeg/Streamlink and long-running services.
- Cloudflare Tunnel is also a good free HTTPS front door if you already have a domain in Cloudflare. Use the same public URLs and point the tunnel to Caddy or directly to service ports.
- Twitch OAuth redirect URLs must match exactly, including scheme, host, path, and trailing slash behavior.
