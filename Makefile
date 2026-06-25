GO ?= go
GO_DOCKER_IMAGE ?= golang:1.25-alpine
VERSION ?= $(shell tr -d '[:space:]' < VERSION 2>/dev/null || echo latest)
ENV_FILE ?= .env
COMPOSE_FEATURE_PROFILES ?= $(shell bash -c 'source scripts/lib/env.sh 2>/dev/null; env_feature_compose_profiles "$(ENV_FILE)"' | awk '{for (i=1;i<=NF;i++) printf " --profile %s", $$i}')
COMPOSE_CORE ?= docker compose --env-file $(ENV_FILE) -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml$(COMPOSE_FEATURE_PROFILES)
COMPOSE_SCRAPER ?= $(COMPOSE_CORE) --profile scraper
COMPOSE_FULL ?= $(COMPOSE_CORE) --profile scraper
HELM ?= helm
HELM_RELEASE ?= streamclone-pulse
HELM_CHART ?= charts/pulse
HELM_NAMESPACE ?= streamclone
HELM_LOCAL_VALUES ?= deploy/env/helm-local.yaml
HELM_EXAMPLE_VALUES ?= deploy/env/helm-local.example.yaml
POWERSHELL ?= powershell.exe
TWITCH_SCOPES ?= chat:read chat:edit user:read:follows clips:edit
TWITCH_LOCAL_AUTH_URL ?= http://localhost:8090
TWITCH_ACTION ?= sync
TWITCH_CLIP_LOGIN ?= sodapoppin
CLIPPER_PYTHON ?= python3
PROFILE ?= core
PORTS ?= 8090

CODEGRAPH_VENV ?= .codegraph/.venv
CODEGRAPH_PY ?= $(CODEGRAPH_VENV)/bin/python
CODEGRAPH_DB ?= .codegraph/streamclone.kuzu

ENV_RELOAD_SERVICES ?= chat metadata analytics emote storygraph frontend

.PHONY: help env up app stop down down-clean nuke restart rebuild up-scraper up-full \
	refresh-auth reload-env reload-env-if-stale ensure-oauth ensure-clipper-auth ensure-frontend-config \
	scraper-reload scraper-check scraper-preflight scraper-warm scraper-proxy-benchmark scraper-turnstile-benchmark flame-proxy-preflight flame-proxy-benchmark social-probe hybrid-preflight ps ports migrate logs sync-pulse-chart \
	helm-kubeconfig helm-up helm-down helm-status helm-lint \
	helm-grafana helm-grafana-stop helm-influx helm-influx-stop helm-open \
	helm-pulse-wire helm-pulse-check helm-pulse helm-pulse-sync-token helm-pulse-watch \
	pulse pulse-on pulse-off pulse-check pulse-down pulse-watch \
	test vet build tidy integration-up integration-down integration-test \
	test-video test-analytics test-emote test-storygraph test-metadata \
	test-pulse-emote rebuild-analytics-emote restart-analytics \
	smoke-pulse-emote pulse-emote-pick-stream smoke-pulse-emote-gold smoke-pulse-emote-gold-fail \
	go-test-docker go-vet-docker go-build-docker \
	twitch twitch-debug twitch-sync twitch-local-auth clipper-refresh-token \
	clipper-test clipper-restart codegraph-install codegraph codegraph-full codegraph-smoke codegraph-incremental codegraph-mcp mcp-setup codex-setup codex-sync-skills \
	context-snapshots context-verify \
	docs-screenshots docs-media frontend-build frontend-test frontend-audit \
	frontend-restart frontend-refresh frontend-logs compose-config-check azure-scraper-config-check azure-archive-plane-config-check bearhost-config-check bearhost-config-check-local bearhost-config-check-release bearhost-rsync bearhost bearhost-help bearhost-bronze-status bearhost-corpus-only grafana grafana-up grafana-stop grafana-setup grafana-sync grafana-watch grafana-archive-status grafana-watch-install grafana-watch-install-cron grafana-watch-uninstall grafana-watch-uninstall-cron bearhost-grafana bearhost-grafana-up bearhost-grafana-stop bearhost-grafana-setup bearhost-grafana-tunnel bearhost-grafana-tunnel-start bearhost-grafana-tunnel-stop bearhost-grafana-sync bearhost-observability-enable bearhost-observability-status bearhost-observability-up bearhost-observability-down local-vps-only check check-quick \
	bootstrap setup validate-env security-scan smoke smoke-ui install-hooks \
	preflight-deps start stop-user ensure-localhost agent-smoke coverage-report \
	rebuild-analytics-emote restart-analytics test-pulse-emote \
	smoke-pulse-emote pulse-emote-pick-stream smoke-pulse-emote-gold smoke-pulse-emote-gold-fail \
	laptopworker-status laptopworker-smoke laptopworker-update laptopworker-boot-check laptopworker-setup laptopworker-setup-verify

help:
	@printf 'Streamclone — common targets\n\n'
	@printf 'Stack:\n'
	@printf '  make up / app        Start core stack\n'
	@printf '  make up-scraper      Core + TwitchTracker scraper\n'
	@printf '  make up-full         Core + Analytics scraper\n'
	@printf '  make stop / down     Stop compose (keep data)\n'
	@printf '  make local-vps-only  Stop local scraper; disable Tier-0/Bronze (VPS owns scrape)\n'
	@printf '  make down-clean      Stop + remove pg/minio/clipper/influx/grafana volumes\n'
	@printf '  make nuke            Full teardown: compose (all profiles), helm pulse, setup-control, orphans\n'
	@printf '  make restart         stop + up\n'
	@printf '  make rebuild         stop + up-full\n'
	@printf '  make ps / ports / logs / migrate\n\n'
	@printf 'Auth:\n'
	@printf '  make refresh-auth        OAuth sync + reload stale services\n'
	@printf '  make twitch-local-auth   Device-code login for localhost:8090\n'
	@printf '  make twitch-sync         Sync Twitch CLI creds into .env\n\n'
	@printf 'Helm (Emote Pulse): .local/helm-pulse/README.md\n'
	@printf '  make pulse           First-time: deploy k8s + wire compose + port-forwards\n'
	@printf '  make pulse-on        Probe localhost (LoadBalancer) or restart port-forwards\n'
	@printf '  make pulse-check     Verify pods, forwards, env, and Influx data\n'
	@printf '  make pulse-off       Stop port-forwards only\n'
	@printf '  make pulse-down      Stop forwards + uninstall k8s release\n\n'
	@printf 'Quality: make check-quick | make check | test | vet | build | clipper-test | smoke | agent-smoke\n'
	@printf 'Pulse emote (A+ path): make rebuild-analytics-emote | test-pulse-emote | smoke-pulse-emote\n'
	@printf '  make pulse-emote-pick-stream | smoke-pulse-emote-gold LOGIN=... STREAM_ID=...\n'
	@printf 'Agent MCP: make mcp-setup | make codex-setup | codegraph | bash scripts/mcp-preflight.sh\n'
	@printf '          make test-video | test-analytics | test-emote | test-storygraph | test-metadata\n'
	@printf '          make frontend-test | frontend-audit | compose-config-check\n'
	@printf '          make frontend-refresh  Build + migrate + restart frontend/chat/analytics\n'
	@printf '          make frontend-restart  Rebuild frontend image + restart frontend/proxy only\n\n'
	@printf 'Laptopworker (tailnet dev host — run from repo root):\n'
	@printf '  make laptopworker-status   ssh stack ps on laptopworker\n'
	@printf '  make laptopworker-smoke      remote health smoke\n'
	@printf '  make laptopworker-update     git pull + compose on laptop after push\n'
	@printf '  make laptopworker-boot-check linger, systemd, smoke (remote)\n'
	@printf '  make laptopworker-setup       one-shot QoL (sudo once at desk)\n'
	@printf '  make laptopworker-setup-verify  confirm ufw, linger, sudoers\n'
	@printf '  Without make: scripts\\laptopworker-remote.cmd status|smoke|update\n\n'
	@printf 'legacy-rollback-host (corpus + Grafana — from your PC):\n'
	@printf '  make bearhost-help              List BearHost make targets\n'
	@printf '  make local-vps-only             Stop local scraper; VPS owns scrape\n'
	@printf '  make bearhost-bronze-status     Bronze/VOD job summary (SSH to VPS)\n'
	@printf '  make grafana-setup              First time: start Prometheus/Grafana on VPS\n'
	@printf '  make grafana-up                 SSH tunnel → http://localhost:3001\n'
	@printf '  make grafana-stop               Stop Grafana SSH tunnel\n'
	@printf '  make grafana-sync               Push dashboard edits to VPS\n'
	@printf '  make bearhost-rsync             Push repo checkout to VPS\n'

env:
	@test -f .env || cp .env.dev .env

up app: env ensure-oauth
	$(COMPOSE_CORE) up -d --build --remove-orphans
	@$(MAKE) reload-env-if-stale
	@$(MAKE) ensure-localhost PORTS=8090

up-scraper: env ensure-oauth
	$(COMPOSE_SCRAPER) up -d --build --remove-orphans
	@$(MAKE) reload-env-if-stale
	@$(MAKE) ensure-localhost PORTS=8090
	@if [ -z "$$SCRAPER_SKIP_PREFLIGHT" ]; then $(MAKE) scraper-preflight; fi

up-full: env ensure-oauth
	$(COMPOSE_FULL) up -d --build --remove-orphans
	@$(MAKE) reload-env-if-stale
	@$(MAKE) ensure-localhost PORTS=8090
	@if [ -z "$$SCRAPER_SKIP_PREFLIGHT" ]; then $(MAKE) scraper-preflight; fi

stop down:
	@command -v bash >/dev/null 2>&1 && ENV_FILE=$(ENV_FILE) bash scripts/compose-down.sh || \
		wsl bash -lc "cd '$$(wslpath -a '$(CURDIR)')' && ENV_FILE='$(ENV_FILE)' bash scripts/compose-down.sh"

down-clean:
	@command -v bash >/dev/null 2>&1 && ENV_FILE=$(ENV_FILE) bash scripts/compose-down.sh --volumes || \
		wsl bash -lc "cd '$$(wslpath -a '$(CURDIR)')' && ENV_FILE='$(ENV_FILE)' bash scripts/compose-down.sh --volumes"

nuke:
	@command -v bash >/dev/null 2>&1 && ENV_FILE=$(ENV_FILE) bash scripts/nuke.sh || \
		wsl bash -lc "cd '$$(wslpath -a '$(CURDIR)')' && ENV_FILE='$(ENV_FILE)' bash scripts/nuke.sh"

restart: stop up
rebuild: stop up-full

reload-env: env
	@echo "Recreating env-sensitive services ($(ENV_RELOAD_SERVICES))..."
	$(COMPOSE_CORE) up -d --no-deps --force-recreate $(ENV_RELOAD_SERVICES)

reload-env-if-stale: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/reload-env-if-stale.ps1 -EnvFile $(ENV_FILE)

ensure-localhost:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-localhost-relays.ps1 -Ports "$(PORTS)" || true

ensure-oauth: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-oauth-env.ps1 -EnvFile $(ENV_FILE) || true

ensure-clipper-auth: env
	@echo "ensure-clipper-auth is deprecated — Clip Studio runs in ReplayForge, not Streamclone compose."
	@echo "See docs/agents-streamclone-and-replayforge.md"

ensure-frontend-config: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-frontend-config.ps1 -EnvFile $(ENV_FILE) || true

refresh-auth: env ensure-oauth reload-env-if-stale

scraper-reload: env
	$(COMPOSE_SCRAPER) up -d --no-deps --force-recreate scraper

scraper-check: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/scraper-preflight.ps1 -CheckOnly

scraper-preflight: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/scraper-preflight.ps1

hybrid-preflight: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/azure-hybrid-smoke.ps1 -PreflightOnly
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/scraper-preflight.ps1 -ScraperURL http://azure-streamclone:8000 -CheckOnly

scraper-warm:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/warm-camoufox-profile.ps1

scraper-proxy-benchmark:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/scraper-proxy-benchmark.ps1

scraper-turnstile-benchmark:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/scraper-turnstile-benchmark.ps1

flame-proxy-preflight:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/flame-proxy-preflight.ps1

flame-proxy-benchmark:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/run-flame-proxy-benchmark.ps1 -UseFlameApi -RotateSessionOnFail -RecreateScraperOnRetry

social-probe:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/social-probe.ps1

ps: env
	@docker ps -a --filter "name=streamclone" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

ports:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/stack-ports.ps1

migrate: env
	$(COMPOSE_CORE) run --rm migrate

logs: env
	$(COMPOSE_CORE) logs -f

helm-kubeconfig:
	@bash scripts/helm-preflight.sh

sync-pulse-chart:
	@bash scripts/sync-pulse-chart.sh

helm-up: helm-kubeconfig sync-pulse-chart
	@values="-f $(HELM_CHART)/values.yaml"; \
	if [ -f "$(HELM_LOCAL_VALUES)" ]; then values="$$values -f $(HELM_LOCAL_VALUES)"; \
	elif [ -f "$(HELM_EXAMPLE_VALUES)" ]; then values="$$values -f $(HELM_EXAMPLE_VALUES)"; fi; \
	$(HELM) upgrade --install $(HELM_RELEASE) $(HELM_CHART) \
		-n $(HELM_NAMESPACE) --create-namespace $$values --wait --timeout 10m
	@HELM_NAMESPACE=$(HELM_NAMESPACE) bash scripts/helm-pulse-sync-token.sh
	@HELM_NAMESPACE=$(HELM_NAMESPACE) bash scripts/helm-portforward.sh start all
	@$(MAKE) ensure-localhost PORTS=3000
	@printf '\nGrafana: http://localhost:3000/d/streamclone-emote-pulse/emote-pulse?from=now-7d&to=now (admin / devpulse)\n'
	@printf 'Ops:     http://localhost:3000/d/streamclone-ops/streamclone-ops\n'
	@printf 'Prometheus: http://localhost:9090\n'
	@printf 'Persistent on Docker Desktop: LoadBalancer → localhost (no port-forward tunnel).\n'

helm-down:
	-$(HELM) uninstall $(HELM_RELEASE) -n $(HELM_NAMESPACE)

helm-status:
	kubectl -n $(HELM_NAMESPACE) get pods,svc

helm-lint:
	$(HELM) lint $(HELM_CHART)

helm-grafana: helm-kubeconfig
	@HELM_NAMESPACE=$(HELM_NAMESPACE) bash scripts/helm-portforward.sh start grafana
	@printf 'Grafana: http://localhost:3000 (admin / devpulse)\n'

helm-grafana-stop:
	@HELM_NAMESPACE=$(HELM_NAMESPACE) bash scripts/helm-portforward.sh stop grafana

helm-influx: helm-kubeconfig
	@HELM_NAMESPACE=$(HELM_NAMESPACE) bash scripts/helm-portforward.sh start influx
	@printf 'InfluxDB: http://localhost:18086 (override: PULSE_INFLUX_LOCAL_PORT)\n'

helm-influx-stop:
	@HELM_NAMESPACE=$(HELM_NAMESPACE) bash scripts/helm-portforward.sh stop influx

helm-pulse-wire: helm-kubeconfig
	@ENV_FILE=$(ENV_FILE) HELM_NAMESPACE=$(HELM_NAMESPACE) HELM_RELEASE=$(HELM_RELEASE) \
		PULSE_INFLUX_DOCKER_PORT=$${PULSE_INFLUX_DOCKER_PORT:-18087} \
		bash scripts/helm-pulse-wire.sh

helm-pulse-sync-token: helm-kubeconfig
	@HELM_NAMESPACE=$(HELM_NAMESPACE) HELM_RELEASE=$(HELM_RELEASE) \
		bash scripts/helm-pulse-sync-token.sh

helm-pulse-check: helm-kubeconfig
	@ENV_FILE=$(ENV_FILE) HELM_NAMESPACE=$(HELM_NAMESPACE) HELM_RELEASE=$(HELM_RELEASE) \
		bash scripts/helm-pulse-check.sh

helm-pulse: helm-up helm-pulse-wire
	@$(MAKE) ensure-localhost PORTS=3000,8090

helm-pulse-on: helm-kubeconfig
	@HELM_NAMESPACE=$(HELM_NAMESPACE) bash scripts/helm-portforward.sh start all
	@$(MAKE) ensure-localhost PORTS=3000,8090
	@printf '\nGrafana: http://localhost:3000/d/streamclone-emote-pulse/emote-pulse?from=now-7d&to=now (admin / devpulse)\n'
	@printf 'Ops:     http://localhost:3000/d/streamclone-ops/streamclone-ops\n'
	@printf 'Prometheus: http://localhost:9090\n'
	@printf 'Docker Desktop: LoadBalancer on localhost — no tunnel needed when probe succeeds.\n'

helm-pulse-watch: helm-kubeconfig
	@HELM_NAMESPACE=$(HELM_NAMESPACE) bash scripts/helm-portforward-watch.sh

helm-pulse-off:
	@HELM_NAMESPACE=$(HELM_NAMESPACE) bash scripts/helm-portforward.sh stop all

helm-pulse-down: helm-pulse-off helm-down

# Short aliases (preferred)
pulse: helm-pulse
pulse-on: helm-pulse-on
pulse-watch: helm-pulse-watch
pulse-off: helm-pulse-off
pulse-check: helm-pulse-check
pulse-down: helm-pulse-down

helm-open: helm-grafana
	@printf 'Open http://localhost:3000/d/streamclone-emote-pulse/emote-pulse\n'

go-test-docker:
	docker run --rm -v "$(CURDIR):/src" -w /src $(GO_DOCKER_IMAGE) go test ./...

go-vet-docker:
	docker run --rm -v "$(CURDIR):/src" -w /src $(GO_DOCKER_IMAGE) go vet ./...

go-build-docker:
	docker run --rm -v "$(CURDIR):/src" -w /src $(GO_DOCKER_IMAGE) go build ./...

test:
	@command -v $(GO) >/dev/null 2>&1 && $(GO) test ./... || $(MAKE) go-test-docker

test-video:
	@command -v $(GO) >/dev/null 2>&1 && $(GO) test ./internal/video/... || docker run --rm -v "$(CURDIR):/src" -w /src $(GO_DOCKER_IMAGE) go test ./internal/video/...

test-analytics:
	@command -v $(GO) >/dev/null 2>&1 && $(GO) test ./internal/analytics/... || docker run --rm -v "$(CURDIR):/src" -w /src $(GO_DOCKER_IMAGE) go test ./internal/analytics/...

test-pulse-emote:
	@command -v $(GO) >/dev/null 2>&1 && $(GO) test ./internal/analytics/... -run 'TestRequireReadyForGold|TestCollectorStartKicks|TestWatchReturns|TestLiveEmote|TestEmoteSync' -count=1 || \
		docker run --rm -v "$(CURDIR):/src" -w /src $(GO_DOCKER_IMAGE) go test ./internal/analytics/... -run 'TestRequireReadyForGold|TestCollectorStartKicks|TestWatchReturns|TestLiveEmote|TestEmoteSync' -count=1

rebuild-analytics-emote: env
	$(COMPOSE_CORE) build analytics emote
	$(COMPOSE_CORE) up -d --no-deps analytics emote
	@$(MAKE) ensure-localhost PORTS=8090

restart-analytics: env
	$(COMPOSE_CORE) restart analytics
	@$(MAKE) ensure-localhost PORTS=8090

smoke-pulse-emote: env
	@bash scripts/pulse-emote-smoke.sh

pulse-emote-pick-stream: env
	@bash scripts/pulse-emote-smoke.sh --pick-stream

smoke-pulse-emote-gold: env
	@test -n "$(STREAM_ID)" || (echo "smoke-pulse-emote-gold: set STREAM_ID= (see make pulse-emote-pick-stream)" >&2; exit 1)
	@bash scripts/pulse-emote-smoke.sh --gold

smoke-pulse-emote-gold-fail: env
	@test -n "$(STREAM_ID)" || (echo "smoke-pulse-emote-gold-fail: set STREAM_ID= (see make pulse-emote-pick-stream)" >&2; exit 1)
	@bash scripts/pulse-emote-smoke.sh --gold-fail

coverage-report:
	@command -v $(GO) >/dev/null 2>&1 && $(GO) run ./cmd/backfill coverage report || docker run --rm -v "$(CURDIR):/src" -w /src --env-file .env $(GO_DOCKER_IMAGE) go run ./cmd/backfill coverage report

test-emote:
	@command -v $(GO) >/dev/null 2>&1 && $(GO) test ./internal/emote/... || docker run --rm -v "$(CURDIR):/src" -w /src $(GO_DOCKER_IMAGE) go test ./internal/emote/...

test-storygraph:
	@command -v $(GO) >/dev/null 2>&1 && $(GO) test ./internal/storygraph/... || docker run --rm -v "$(CURDIR):/src" -w /src $(GO_DOCKER_IMAGE) go test ./internal/storygraph/...

test-metadata:
	@command -v $(GO) >/dev/null 2>&1 && $(GO) test ./internal/metadata/... || docker run --rm -v "$(CURDIR):/src" -w /src $(GO_DOCKER_IMAGE) go test ./internal/metadata/...

integration-up:
	docker compose -f internal/integration/docker-compose.test.yml up -d

integration-down:
	docker compose -f internal/integration/docker-compose.test.yml down -v

integration-test: integration-up
	INTEGRATION=1 $(GO) test ./internal/integration/ -count=1 -timeout 120s

vet:
	@command -v $(GO) >/dev/null 2>&1 && $(GO) vet ./... || $(MAKE) go-vet-docker

build:
	@command -v $(GO) >/dev/null 2>&1 && $(GO) build ./... || $(MAKE) go-build-docker

tidy:
	$(GO) mod tidy

# Prefers ../replayforge/backend tests when sibling checkout exists; falls back to in-repo clipper/.
clipper-test:
	@if [ -d ../replayforge/backend/tests ]; then \
		cd ../replayforge/backend && PYTHONPATH=. $(CLIPPER_PYTHON) -m unittest discover -s tests; \
	else \
		PYTHONPATH=clipper $(CLIPPER_PYTHON) -m unittest discover -s clipper/tests; \
	fi

codegraph-install:
	python3 -m venv $(CODEGRAPH_VENV)
	$(CODEGRAPH_PY) -m pip install -r tools/codegraph/requirements.txt

codegraph:
	@test -x $(CODEGRAPH_PY) || $(MAKE) codegraph-install
	PYTHONPATH=. $(CODEGRAPH_PY) tools/codegraph/codegraph_ingest.py --repo . --db $(CODEGRAPH_DB)

codegraph-full: codegraph

codegraph-smoke:
	@test -x $(CODEGRAPH_PY) || $(MAKE) codegraph-install
	PYTHONPATH=. $(CODEGRAPH_PY) tools/codegraph/smoke.py

codegraph-incremental:
	@test -x $(CODEGRAPH_PY) || $(MAKE) codegraph-install
	PYTHONPATH=. $(CODEGRAPH_PY) tools/codegraph/incremental.py --repo . --db $(CODEGRAPH_DB)

codegraph-mcp:
	PYTHONPATH=. $(CODEGRAPH_PY) tools/codegraph/codegraph_mcp.py --repo "$(CURDIR)" --db "$(CURDIR)/$(CODEGRAPH_DB)"

mcp-setup: codegraph-install codegraph
	@bash scripts/mcp-preflight.sh
	@printf '\nNext: copy .cursor/mcp.recommended.json.example → .cursor/mcp.json (gitignored)\n'
	@printf 'Codex: make codex-setup — see docs/CODEX.md\n'

context-snapshots:
	@bash scripts/context/all.sh

context-verify:
	@bash scripts/context/verify-agent-stack.sh

codex-sync-skills:
	@bash scripts/codex-sync-skills.sh

codex-setup:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/codex-setup.ps1

twitch: env
	@if [ "$(TWITCH_ACTION)" = "sync" ]; then \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action sync-env -EnvFile $(ENV_FILE); \
		$(MAKE) reload-env; \
	elif [ "$(TWITCH_ACTION)" = "local-auth" ]; then \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action sync-env -EnvFile $(ENV_FILE); \
		$(COMPOSE_FULL) up -d --no-deps --force-recreate chat metadata analytics emote local-proxy; \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action local-auth -Scopes "$(TWITCH_SCOPES)" -ChatHttp "$(TWITCH_LOCAL_AUTH_URL)"; \
	elif [ "$(TWITCH_ACTION)" = "refresh-clipper-token" ]; then \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action refresh-clipper-token -EnvFile $(ENV_FILE); \
		echo "Clip Studio runs in ReplayForge (../replayforge on host :8095) — restart ReplayForge to load the refreshed token."; \
	else \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action $(TWITCH_ACTION) -EnvFile $(ENV_FILE) -Scopes "$(TWITCH_SCOPES)" -ChatHttp "$(TWITCH_LOCAL_AUTH_URL)"; \
	fi

twitch-debug: env
	@printf 'Auth debug:\n'
	@curl -fsS $(TWITCH_LOCAL_AUTH_URL)/v1/auth/debug
	@printf '\n\nClip probe (%s):\n' "$(TWITCH_CLIP_LOGIN)"
	@curl -fsS "$(TWITCH_LOCAL_AUTH_URL)/v1/channels/$(TWITCH_CLIP_LOGIN)/clips?limit=1"
	@printf '\n'

twitch-sync:
	@$(MAKE) twitch TWITCH_ACTION=sync

twitch-local-auth:
	@$(MAKE) twitch TWITCH_ACTION=local-auth

clipper-refresh-token:
	@$(MAKE) twitch TWITCH_ACTION=refresh-clipper-token

docs-screenshots:
	cd frontend && npx playwright install chromium && npm run screenshots:readme

docs-media:
	cd frontend && npx playwright install chromium && npm run docs:media

frontend-build:
	cd frontend && npm run build

frontend-test:
	cd frontend && npm test

frontend-audit:
	cd frontend && npm audit --audit-level=high

compose-config-check: env
	$(COMPOSE_CORE) config --quiet
	IMAGE_TAG=$(VERSION) docker compose --env-file $(ENV_FILE) \
		-f deploy/docker-compose.yml \
		-f deploy/docker-compose.local-tunnel.yml \
		-f deploy/docker-compose.release.yml config --quiet
	IMAGE_TAG=$(VERSION) docker compose --env-file $(ENV_FILE) \
		-f deploy/docker-compose.yml \
		-f deploy/docker-compose.observability.yml \
		--profile observability config --quiet
	APP_DOMAIN=streamclone.example.invalid ACME_EMAIL=security@example.invalid docker compose --env-file $(ENV_FILE) \
		-f deploy/docker-compose.yml \
		-f deploy/docker-compose.prod.yml config --quiet

azure-scraper-config-check: env
	IMAGE_TAG=$(VERSION) docker compose --env-file $(ENV_FILE) \
		--env-file deploy/env/profile-azure-scraper.env \
		-f deploy/docker-compose.azure-scraper.yml \
		-f deploy/docker-compose.release.yml config --quiet

azure-archive-plane-config-check: env
	IMAGE_TAG=$(VERSION) docker compose --env-file $(ENV_FILE) \
		--env-file deploy/env/profile-archive.env \
		--env-file deploy/env/profile-azure-workers.env \
		-f deploy/docker-compose.azure-archive-plane.yml \
		-f deploy/docker-compose.release.yml config --quiet

bearhost-config-check-release: env
	APP_DOMAIN=legacy-rollback-host ACME_EMAIL=security@example.invalid PUBLIC_ORIGIN=http://legacy-rollback-host \
	IMAGE_TAG=$(VERSION) docker compose --env-file $(ENV_FILE) \
		--env-file deploy/env/profile-full.env \
		--env-file deploy/env/profile-archive.env \
		--env-file deploy/env/profile-bearhost-prod.env \
		-f deploy/docker-compose.yml \
		-f deploy/docker-compose.release.yml \
		-f deploy/docker-compose.prod.yml \
		-f deploy/docker-compose.bearhost-prod.yml \
		--profile scraper config --quiet

bearhost-config-check-local: env
	APP_DOMAIN=legacy-rollback-host ACME_EMAIL=security@example.invalid PUBLIC_ORIGIN=http://legacy-rollback-host \
	BEARHOST_BUILD_LOCAL=1 SCRAPER_USE_IMAGES=0 IMAGE_TAG=$(VERSION) docker compose --env-file $(ENV_FILE) \
		--env-file deploy/env/profile-full.env \
		--env-file deploy/env/profile-archive.env \
		--env-file deploy/env/profile-bearhost-prod.env \
		-f deploy/docker-compose.yml \
		-f deploy/docker-compose.prod.yml \
		-f deploy/docker-compose.bearhost-prod.yml \
		-f deploy/docker-compose.bearhost-build.yml \
		--profile scraper config --quiet

bearhost-config-check: bearhost-config-check-release bearhost-config-check-local

bearhost-corpus-smoke:
	@bash scripts/bearhost-corpus-smoke.sh

bearhost-help bearhost:
	@printf 'legacy-rollback-host — common targets (see docs/site-links.md)\n\n'
	@printf 'Grafana (VPS archive dashboard):\n'
	@printf '  make grafana-setup    First time only — rsync + Prometheus/Grafana on VPS\n'
	@printf '  make grafana-up       Daily — background SSH tunnel → localhost:3001\n'
	@printf '  make grafana-watch-install      Windows Task Scheduler (every 5 min)\n'
	@printf '  make grafana-watch-install-cron WSL cron fallback (every 5 min)\n'
	@printf '  make grafana-watch              One health check (restart if dead)\n'
	@printf '  make grafana-stop     Stop SSH tunnel on :3000/:3001\n'
	@printf '  make grafana-sync     After editing deploy/grafana/ — push + reload\n'
	@printf '  URL: http://localhost:3001/d/streamclone-archive/streamclone-archive\n'
	@printf '  Login: admin / streampulse\n\n'
	@printf 'Corpus / deploy:\n'
	@printf '  make local-vps-only           Disable local Tier-0/Bronze/scraper\n'
	@printf '  make bearhost-corpus-only     VPS: stop playback/UI; bronze+silver+scraper only\n'
	@printf '  make bearhost-bronze-status   Bronze indexer + backfill summary\n'
	@printf '  make bearhost-rsync           Sync app + scraper trees to VPS\n'
	@printf '  make bearhost-observability-status  Check prometheus-obs / grafana-obs on VPS\n\n'
	@printf 'Legacy aliases still work: bearhost-grafana-tunnel-start, bearhost-observability-enable, …\n'

bearhost-observability-up:
	@bash scripts/bearhost-observability.sh up

bearhost-observability-down:
	@bash scripts/bearhost-observability.sh down

ifeq ($(OS),Windows_NT)
grafana grafana-up bearhost-grafana bearhost-grafana-up bearhost-grafana-tunnel-start:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-grafana-tunnel-start.ps1

grafana-stop bearhost-grafana-stop bearhost-grafana-tunnel-stop:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-grafana-tunnel-stop.ps1
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-wsl-run.ps1 -Script bearhost-grafana-tunnel-stop.sh

grafana-setup bearhost-grafana-setup bearhost-observability-enable:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-observability-enable-remote.ps1

grafana-sync bearhost-grafana-sync:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-wsl-run.ps1 -Script bearhost-grafana-sync-remote.sh

grafana-watch:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-grafana-tunnel-watch.ps1

grafana-archive-status:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-grafana-tunnel-watch.ps1
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-wsl-run.ps1 -Script bearhost-archive-status-via-grafana.sh

grafana-watch-install:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-grafana-tunnel-watch-install.ps1

grafana-watch-uninstall:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-grafana-tunnel-watch-uninstall.ps1

grafana-watch-install-cron:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-wsl-run.ps1 -Script bearhost-grafana-tunnel-watch-install-cron.sh

grafana-watch-uninstall-cron:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-wsl-run.ps1 -Script bearhost-grafana-tunnel-watch-uninstall-cron.sh

bearhost-observability-status:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-wsl-run.ps1 -Script bearhost-observability-status-remote.sh

bearhost-grafana-tunnel:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-grafana-tunnel.ps1

bearhost-bronze-status:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-bronze-status-remote.ps1

bearhost-corpus-only:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-corpus-only-remote.ps1

bearhost-rsync:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-rsync-to-vps.ps1

local-vps-only:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/bearhost-wsl-run.ps1 -Script local-vps-only.sh
else
grafana grafana-up bearhost-grafana bearhost-grafana-up bearhost-grafana-tunnel-start:
	@bash scripts/bearhost-grafana-tunnel-start.sh

grafana-stop bearhost-grafana-stop bearhost-grafana-tunnel-stop:
	@bash scripts/bearhost-grafana-tunnel-stop.sh

grafana-setup bearhost-grafana-setup bearhost-observability-enable:
	@bash scripts/bearhost-observability-enable-remote.sh

grafana-sync bearhost-grafana-sync:
	@bash scripts/bearhost-grafana-sync-remote.sh

grafana-watch:
	@bash scripts/bearhost-grafana-tunnel-watch.sh

grafana-archive-status:
	@bash scripts/bearhost-grafana-tunnel-watch.sh
	@bash scripts/bearhost-archive-status-via-grafana.sh

grafana-watch-install:
	@printf 'grafana-watch-install: Windows Task Scheduler only.\n'
	@printf 'Use: make grafana-watch-install-cron   (WSL cron, every 5 min)\n'

grafana-watch-uninstall:
	@bash scripts/bearhost-grafana-tunnel-watch-uninstall-cron.sh

grafana-watch-install-cron:
	@bash scripts/bearhost-grafana-tunnel-watch-install-cron.sh

grafana-watch-uninstall-cron:
	@bash scripts/bearhost-grafana-tunnel-watch-uninstall-cron.sh

bearhost-observability-status:
	@bash scripts/bearhost-observability-status-remote.sh

bearhost-grafana-tunnel:
	@bash scripts/bearhost-grafana-tunnel.sh

bearhost-bronze-status:
	@bash scripts/bearhost-bronze-status-remote.sh

bearhost-corpus-only:
	@bash scripts/bearhost-corpus-only-remote.sh

bearhost-rsync:
	@python3 -c "import pathlib; f=pathlib.Path('scripts/bearhost-rsync-to-vps.sh'); f.write_text(f.read_text().replace('\r\n','\n').replace('\r','\n'), encoding='utf-8')" 2>/dev/null || true
	@bash scripts/bearhost-rsync-to-vps.sh

local-vps-only:
	@bash scripts/local-vps-only.sh
endif

bearhost-cron-install:
	@bash scripts/bearhost-cron-install.sh

archive-restore-drill:
	@bash scripts/archive-restore-drill.sh

check-quick: test vet frontend-test compose-config-check

check: security-scan frontend-build frontend-audit frontend-test clipper-test test vet build compose-config-check

frontend-restart: env
	$(COMPOSE_CORE) build frontend
	$(COMPOSE_CORE) up -d --no-deps --force-recreate frontend local-proxy
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-setup-control.ps1
	@$(MAKE) ensure-localhost PORTS=8090

# Rebuild frontend assets, apply DB migrations, and recreate frontend + chat + analytics
# (chat logs, mod events, linkify). Use after pulling chat/logs UI changes.
frontend-refresh: env frontend-build
	@echo "Applying migrations..."
	$(COMPOSE_CORE) run --rm migrate
	@echo "Rebuilding frontend, chat, and analytics..."
	$(COMPOSE_CORE) build frontend chat analytics
	$(COMPOSE_CORE) up -d --no-deps --force-recreate frontend chat analytics local-proxy
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-setup-control.ps1
	@$(MAKE) ensure-localhost PORTS=8090

clipper-restart:
	@echo "clipper-restart is deprecated — manage ReplayForge separately (../replayforge)."
	@echo "See docs/agents-streamclone-and-replayforge.md"
	@exit 1

frontend-logs: env
	$(COMPOSE_CORE) logs -f frontend

bootstrap:
	@bash scripts/bootstrap.sh

laptopworker-status:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/laptopworker-remote.ps1 status

laptopworker-smoke:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/laptopworker-remote.ps1 smoke

laptopworker-update:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/laptopworker-remote.ps1 update

laptopworker-boot-check:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/laptopworker-remote.ps1 boot-check

laptopworker-setup:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/laptopworker-remote.ps1 setup

laptopworker-setup-verify:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/laptopworker-remote.ps1 setup-verify

setup:
	@bash scripts/setup.sh

validate-env:
	@bash scripts/validate-env.sh --profile $(PROFILE)

smoke:
	@bash scripts/smoke-core.sh

smoke-ui:
	@bash scripts/smoke-core.sh --ui

install-hooks:
	@command -v pre-commit >/dev/null 2>&1 || { echo "Install pre-commit: pip install pre-commit"; exit 1; }
	pre-commit install

security-scan:
	@bash scripts/security-scan.sh

agent-smoke:
	@bash scripts/agent-smoke.sh

preflight-deps:
	@bash scripts/preflight-deps.sh --install-hints

start:
	@bash scripts/start-streamclone.sh

stop-user:
	@bash scripts/stop-streamclone.sh
