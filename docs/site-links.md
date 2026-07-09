# Streamclone — quick links

Local/self-hosted links for the Twitch replica. **StreamPulse** (hosted extension + portal) is a separate product — see [streampulse-product-boundary.md](streampulse-product-boundary.md).

## Site

| What | URL |
|------|-----|
| Local dev | http://localhost:8090 |
| GitHub | https://github.com/Aron-Chu/streamclone |

## Login

| Where | Works? |
|-------|--------|
| **localhost:8090** | Yes — “Sign in with Twitch” uses loopback-only dev/device auth |

## Local development

```sh
make setup
make up
make smoke
```

See [install-desktop.md](install-desktop.md) and [ENVIRONMENT.md](ENVIRONMENT.md).

## Hosted / operator work

Deploy runbooks, production contracts, and host topology live in private **streampulse-ops** and **streampulse-backend** — not in this public repository. See [streampulse-product-boundary.md](streampulse-product-boundary.md).
