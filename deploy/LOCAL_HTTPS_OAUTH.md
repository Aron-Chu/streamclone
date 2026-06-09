# Local HTTPS Tunnel

Use this when you want to keep Streamclone running on your PC but expose it over public HTTPS for remote-device testing.

If you only need the app on the same machine, skip the tunnel and use `http://localhost:8090` plus the local device-code login flow. That keeps browser, chat auth, HLS, and WebSocket traffic local and is the lowest-latency development path.

Redirect OAuth login was removed. You do **not** need to register Twitch OAuth redirect URLs for tunnel or DuckDNS deployments. Viewers can browse anonymously; optional chat login remains localhost-only.

The local app stays in Docker. A small local Caddy proxy listens on:

```text
http://localhost:8090
```

Then a public tunnel exposes that local proxy as public HTTPS.

## Recommended: Cloudflare Quick Tunnel

Cloudflare Quick Tunnel does not inject a browser warning page, so browser fetches, HLS manifests, HLS segments, and WebSocket upgrades all reach the app directly.

1. Start a quick tunnel:

```sh
cloudflared tunnel --url http://localhost:8090
```

2. Copy the generated URL, for example:

```text
https://random-words.trycloudflare.com
```

3. Set:

```env
PUBLIC_ORIGIN=https://random-words.trycloudflare.com
PUBLIC_ORIGIN_WS=wss://random-words.trycloudflare.com
FRONTEND_ORIGIN=https://random-words.trycloudflare.com
HLS_PUBLIC_BASE=https://random-words.trycloudflare.com
CDN_PUBLIC_BASE=https://random-words.trycloudflare.com
AUTH_COOKIE_SAMESITE=lax
```

4. Start or restart the stack:

```sh
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml up -d --build
```

## Free ngrok caveat

Free ngrok dev domains inject a browser warning/interstitial unless each request sends the `ngrok-skip-browser-warning` header. Browser `fetch` calls can add that header, but native browser WebSocket connections cannot, and HLS playback is unreliable because manifests and segments are fetched outside normal app code.

Typical symptoms are:

- `ERR_NGROK_6024` or a plain-text warning page instead of JSON/HTML.
- HLS manifests or segments returning `401`, `404`, or warning-page HTML.
- browser WebSocket connections to `/v1/ws` failing before they reach the chat service.

**Note:** MediaMTX 1.18+ emits `cookieCheck=1` redirects on `index.m3u8` as part of its HLS session system. On plain HTTP localhost this breaks playback unless Caddy injects the MediaMTX CDN secret — see `.kiro/steering/playback.md` (`hlsCDNSecret` + `Authorization: Bearer` on `/live/*` routes). Tunnels over HTTPS may behave differently; keep `PUBLIC_ORIGIN` aligned with the tunnel URL.

If you want to use ngrok, use a setup that does not serve the browser warning page for app traffic.

## Alternate Option: ngrok Dev Domain

ngrok's free plan includes one assigned dev domain. A stable URL is useful for repeated remote testing, but the free warning page makes it a poor fit for full browser playback/chat testing.

1. Install ngrok and sign in.
2. In ngrok dashboard, find your free static/dev domain, for example:

```text
your-name.ngrok-free.app
```

3. Set these in `.env`:

```env
PUBLIC_ORIGIN=https://your-name.ngrok-free.app
PUBLIC_ORIGIN_WS=wss://your-name.ngrok-free.app
FRONTEND_ORIGIN=https://your-name.ngrok-free.app
HLS_PUBLIC_BASE=https://your-name.ngrok-free.app
CDN_PUBLIC_BASE=https://your-name.ngrok-free.app
AUTH_COOKIE_SAMESITE=lax
```

4. Start the local stack:

```sh
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml up -d --build
```

5. Start ngrok:

```sh
ngrok http --domain=your-name.ngrok-free.app 8090
```

6. Open:

```text
https://your-name.ngrok-free.app
```

## Smoke Tests

Replace the URL with your tunnel:

```sh
curl https://your-name.ngrok-free.app/v1/auth/debug
curl https://your-name.ngrok-free.app/
```

Expected:

- `/v1/auth/debug` shows `ready: true` when Twitch client id/secret are configured.
- The directory loads and channels are browsable without login.
- API endpoints should return JSON, not the ngrok warning page.

## Important

- If you change tunnel domains, update `PUBLIC_ORIGIN` / `PUBLIC_ORIGIN_WS` and restart Docker Compose.
- Optional chat login via `Use local token` only works on loopback (`localhost:8090`), not through a public tunnel URL.
