## Summary

<!-- What changed and why? -->

## Checklist

- [ ] CI is green on this branch
- [ ] No `.env`, tokens, or secrets committed
- [ ] No raw `docker compose config`, env dumps, setup-control diagnostics, or token-bearing logs pasted into the PR
- [ ] `make security-scan`
- [ ] `make frontend-build` and `make frontend-audit`
- [ ] `make frontend-test` when frontend logic changed
- [ ] `make test` and `make vet` (or Docker-backed Go fallback) when Go code changed
- [ ] `make compose-config-check` when compose, env templates, or deploy overlays changed
- [ ] `make clipper-test` when clipper code changed
- [ ] Install/benchmark scripts re-run if installer or compose overlays changed

## Test plan

<!-- Commands run and what you verified manually -->
