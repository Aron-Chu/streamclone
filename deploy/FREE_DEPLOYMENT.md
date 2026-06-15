# Public VM Deployment

Streamclone needs a VM, not static hosting. It runs a React frontend, Go services, Redis, PostgreSQL, MinIO, MediaMTX, Streamlink, and FFmpeg.

Use this as a hardening checklist, not a managed hosting promise.

## Recommended Shape

- Ubuntu VM with Docker Compose
- DNS name pointing at the VM
- Caddy on `80/443`
- `deploy/docker-compose.yml` plus `deploy/docker-compose.prod.yml`

Open only:

- `22/tcp` from your IP
- `80/tcp` for Let's Encrypt
- `443/tcp` for the app

Never expose Redis, Postgres, MinIO, MediaMTX, setup-control, or raw service ports.

## Setup

```sh
sudo apt update
sudo apt install -y ca-certificates curl git
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker "$USER"
newgrp docker

git clone https://github.com/Aron-Chu/streamclone.git
cd streamclone
cp .env.example .env
```

Set production values in `.env`:

```env
APP_DOMAIN=your.domain.example
ACME_EMAIL=you@example.com
FRONTEND_ORIGIN=https://your.domain.example
PUBLIC_ORIGIN=https://your.domain.example
PUBLIC_ORIGIN_WS=wss://your.domain.example
HLS_PUBLIC_BASE=https://your.domain.example
CDN_PUBLIC_BASE=https://your.domain.example
TWITCH_DEV_TOKEN_IMPORT_ENABLED=false

AUTH_COOKIE_SECRET=replace-with-random
CURATOR_API_TOKEN=replace-with-random
CLIPPER_WEBHOOK_TOKEN=replace-with-random
MINIO_ROOT_USER=replace-minio-user
MINIO_ROOT_PASSWORD=replace-minio-password
S3_ACCESS_KEY=replace-minio-user
S3_SECRET_KEY=replace-minio-password
```

Generate secrets:

```sh
openssl rand -hex 32
```

Start:

```sh
docker compose --env-file .env \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.prod.yml \
  up -d --build
```

Smoke test:

```sh
curl -I https://your.domain.example
curl https://your.domain.example/v1/auth/debug
```

## Local HTTPS Tunnel

For remote-device testing from your PC, tunnel only `http://localhost:8090`.

Cloudflare Quick Tunnel usually works better than free ngrok because it does not inject a browser warning page into API, HLS, or WebSocket traffic.

Required env shape for a tunnel:

```env
PUBLIC_ORIGIN=https://your-tunnel.example
PUBLIC_ORIGIN_WS=wss://your-tunnel.example
FRONTEND_ORIGIN=https://your-tunnel.example
HLS_PUBLIC_BASE=https://your-tunnel.example
CDN_PUBLIC_BASE=https://your-tunnel.example
TWITCH_DEV_TOKEN_IMPORT_ENABLED=false
```

Optional chat token import is localhost-only. Do not expose setup-control or clipper mutation tokens unless every visitor is trusted.

## Notes

- Anonymous watching does not need Twitch login.
- Twitch client credentials improve Helix enrichment and token refresh.
- Free cloud capacity changes; stay inside provider limits.
- Read [docs/security.md](../docs/security.md) before exposing the stack.
