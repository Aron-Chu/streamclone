# Hosted production (private ops)

Hosted production execution (deploy, secrets, smoke, rollback evidence) lives in **private streampulse-ops** — never commit operator runbooks or host topology to this public repository.

## Public contract

- [hosted-production-ops.md](hosted-production-ops.md)
- [production-artifact-contract.md](production-artifact-contract.md)
- [production-promotion-contract.md](production-promotion-contract.md)

## Public API (safe probes)

```bash
curl -fsS https://api.streampulse.stream/v1/extension/health
bash scripts/hosted-launch-probes.sh
```

Internal ops routes, SSH, and VPS shell checks belong in **private streampulse-ops** only.
