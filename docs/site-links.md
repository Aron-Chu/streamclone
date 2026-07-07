# Streamclone — quick links

Hosted production ops are maintained in private **streampulse-ops**.
This public repo contains only local/self-hosted examples.

## Site

| What | URL |
|------|-----|
| Local dev | http://localhost:8090 |
| GitHub | https://github.com/Aron-Chu/streamclone |
| Production API | https://api.streampulse.stream (hosted; ops in private repo) |

## Login

| Where | Works? |
|-------|--------|
| **localhost:8090** | Yes — “Sign in with Twitch” uses loopback-only dev/device auth |
| **Hosted production** | See StreamPulse portal / Twitch OAuth requirements in `docs/multi-user/requirements.md` |

## Local development

```sh
make setup
make up
make smoke
```

See [docs/install-desktop.md](install-desktop.md) and [docs/ENVIRONMENT.md](ENVIRONMENT.md).

## Production operations

Operator runbooks, deploy scripts, and host topology live in the private **streampulse-ops** repository.
See [docs/hosted-production-ops.md](hosted-production-ops.md), [docs/production-promotion-contract.md](production-promotion-contract.md), and [docs/production-artifact-contract.md](production-artifact-contract.md).
