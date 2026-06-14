## Checklist

- [ ] CI is green on this branch
- [ ] No `.env` or secrets committed
- [ ] No raw `docker compose config`, env dumps, setup-control diagnostics, or token-bearing logs pasted into the PR
- [ ] If compose, auth, clipper, setup-control, tunnel, or public deploy behavior changed: `make security-scan` and relevant compose `config --quiet` checks were run
- [ ] Install/benchmark scripts re-run if installer or compose overlays changed
