# Deployment

## Observability

Each Go service exposes `/healthz`, `/readyz`, and `/metrics`. A Compose overlay adds Prometheus, Grafana, Loki, Promtail, cAdvisor, node-exporter, Redis/PostgreSQL exporters, blackbox readiness probes, and MinIO/MediaMTX scrapes:

```sh
make obs-up
```

| Surface | URL | Purpose |
|---|---|---|
| Grafana | `http://localhost:3001` | Provisioned Streamclone operator dashboard and Loki logs |
| Prometheus | `http://localhost:9090` | Targets, PromQL, and alert state |
| Loki | `http://localhost:3100` | Log API used by Grafana |
| cAdvisor | `http://localhost:8085` | Container CPU, memory, filesystem, and network metrics |

Default Grafana credentials come from `GRAFANA_ADMIN_USER` / `GRAFANA_ADMIN_PASSWORD` in `.env`; the example defaults are `admin` / `admin` for local-only use.

Useful checks:

```sh
make obs-config
curl http://localhost:9090/-/ready
curl http://localhost:3001/api/health
```

The provisioned dashboard tracks service health, readiness failures, HTTP latency/error rates, active streams, per-channel listeners, stream start failures, HLS readiness, chat throughput, cache results, upstream failures, emote asset jobs, storage usage, CPU/memory, and network ingress/egress.

Stop the observability stack with `make obs-down`.

## Public HTTPS deployment

For a public HTTPS deployment, use a VM with Docker Compose and a single HTTPS origin:

```sh
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml up -d --build
```

Full walkthrough: [deploy/FREE_DEPLOYMENT.md](../deploy/FREE_DEPLOYMENT.md).

Viewers can browse and watch without logging in; Twitch app credentials in `.env` are for Helix and optional localhost chat login.

## Local HTTPS tunnel

For remote-device testing over HTTPS:

```sh
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml up -d --build
```

Walkthrough: [deploy/LOCAL_HTTPS_OAUTH.md](../deploy/LOCAL_HTTPS_OAUTH.md).

For localhost-only chat login with the lowest added latency, use the device-code flow in [oauth.md](../oauth.md) instead of a tunnel.

## Sharing and cloud storage

See [DISTRIBUTION.md](../DISTRIBUTION.md) for tiers (local, tunnel, public VM, GCS hybrid).

## Standalone live clipper

The repo includes a separate local clipper app under `clipper/`. It is not part of the Streamclone viewer stack.

```sh
cd clipper
python3 -m venv .venv
. .venv/bin/activate
python -m pip install -r requirements.txt
cd ..
make clipper-run CLIPPER_PYTHON=clipper/.venv/bin/python
```

Open `http://127.0.0.1:8095`. Configure `CLIPPER_TWITCH_CLIENT_ID` and `CLIPPER_TWITCH_USER_ACCESS_TOKEN` before creating real clips.

See [clipper/README.md](../clipper/README.md) for details.

## Security

Before exposing the stack publicly, read [security.md](security.md).
