# Emote Pulse Helm sandbox

The `charts/pulse` chart deploys a **test-only** telemetry stack on Kubernetes: InfluxDB, Grafana (with the Emote Pulse dashboard), and Prometheus. It does **not** deploy Streamclone app services — Postgres, Redis, analytics, and the rest stay on Docker Compose.

## Prerequisites

- [Helm 3](https://helm.sh/docs/intro/install/) CLI
- A working `kubectl` context (see **Docker Desktop** below)
- Streamclone compose stack running locally when you want live data

### Docker Desktop + WSL

1. Open **Docker Desktop** → **Settings** → **Kubernetes** → enable **Kubernetes** and wait until status is **Running**.
2. From your WSL repo shell:

   ```bash
   make helm-kubeconfig   # links Windows ~/.kube/config into WSL
   kubectl config current-context   # should show docker-desktop
   ```

If `make helm-up` fails with `localhost:8080: connection refused`, Kubernetes is not enabled or WSL has no kubeconfig — run the steps above.

Optional local overrides:

```bash
cp deploy/env/helm-local.example.yaml deploy/env/helm-local.yaml
# edit token + Grafana password
```

## Deploy and tear down

```bash
make helm-up      # helm upgrade --install streamclone-pulse charts/pulse
make helm-status  # pods and services in namespace streamclone
make helm-down    # uninstall release
make helm-lint    # chart validation (CI)
```

Grafana is exposed as a **NodePort** by default. After `make helm-up`:

```bash
kubectl -n streamclone get svc streamclone-pulse-grafana
```

Or port-forward:

```bash
kubectl -n streamclone port-forward svc/streamclone-pulse-grafana 3000:3000
kubectl -n streamclone port-forward svc/streamclone-pulse-influxdb 8086:8086
```

Open Grafana → folder **Emote Pulse** → dashboard **Emote Pulse**.

## Live data from compose analytics

The analytics service writes minute rollups to Influx when timeseries export is enabled (`internal/timeseries/writer.go`). No Helm changes are required — point compose at the k8s Influx instance.

1. Port-forward Influx (or use the NodePort if your cluster exposes it to the host):

   ```bash
   kubectl -n streamclone port-forward svc/streamclone-pulse-influxdb 8086:8086
   ```

2. Add to `.env` or `.env.dev` (token must match `influxdb.adminToken` in Helm values / `deploy/env/helm-local.yaml`):

   ```env
   TIMESERIES_ENABLED=true
   INFLUXDB_URL=http://host.docker.internal:8086
   INFLUXDB_TOKEN=<same token as helm-local.yaml>
   INFLUXDB_ORG=streamclone
   INFLUXDB_BUCKET=streamclone
   ```

   On Linux without Docker Desktop, use the host IP or `http://172.17.0.1:8086` instead of `host.docker.internal`.

3. Recreate analytics so it picks up env:

   ```bash
   make reload-env
   ```

4. Watch live panels (`emote_usage_1m`, `stream_activity_1m`) refresh in Grafana.

## Disclaimer

This chart is a **local test sandbox** for Emote Pulse telemetry. It is not a production Streamclone deployment path. Do not use default passwords or tokens outside dev.
