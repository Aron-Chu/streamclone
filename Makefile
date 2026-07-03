GO ?= go
GO_DOCKER_IMAGE ?= golang:1.25-alpine
VERSION ?= $(shell tr -d '[:space:]' < VERSION 2>/dev/null || echo latest)
ENV_FILE ?= .env
COMPOSE_FEATURE_PROFILES ?= $(shell bash -c 'source scripts/lib/env.sh 2>/dev/null; env_feature_compose_profiles "$(ENV_FILE)"' | awk '{for (i=1;i<=NF;i++) printf " --profile %s", $$i}')
COMPOSE_CORE ?= docker compose --env-file $(ENV_FILE) -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml$(COMPOSE_FEATURE_PROFILES)
COMPOSE_SCRAPER ?= $(COMPOSE_CORE) --profile scraper
COMPOSE_FULL ?= $(COMPOSE_CORE) --profile scraper
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
CODEGRAPH_PULSE_REPO ?= ../streamclone-pulse
CODEGRAPH_PULSE_DB ?= $(CODEGRAPH_PULSE_REPO)/.codegraph/streamclone-pulse.kuzu

ENV_RELOAD_SERVICES ?= chat metadata analytics emote frontend

.PHONY: help env up app stop down down-clean nuke restart rebuild up-scraper up-full \
	refresh-auth reload-env reload-env-if-stale ensure-oauth ensure-frontend-config \
	scraper-reload scraper-check scraper-preflight scraper-warm scraper-proxy-benchmark scraper-turnstile-benchmark flame-proxy-preflight flame-proxy-benchmark social-probe hybrid-preflight ps ports migrate logs \
	test vet build tidy integration-up integration-down integration-test \
	test-video test-analytics test-analytics-gold-segments test-emote test-metadata \
	test-pulse-emote rebuild-analytics-emote restart-analytics \
	smoke-pulse-emote pulse-emote-pick-stream smoke-pulse-emote-gold smoke-pulse-emote-gold-fail \
	go-test-docker go-vet-docker go-build-docker \
	twitch twitch-debug twitch-sync twitch-local-auth clipper-refresh-token \
	clipper-test codegraph-install codegraph codegraph-full codegraph-smoke codegraph-incremental codegraph-mcp codegraph-pulse mcp-setup codex-setup codex-sync-skills claude-setup claude-sync-skills claude-sync-agents \
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
	@printf '  make down-clean      Stop + remove pg/minio volumes\n'
	@printf '  make nuke            Full teardown: compose (all profiles), setup-control, orphans\n'
	@printf '  make restart         stop + up\n'
	@printf '  make rebuild         stop + up-full\n'
	@printf '  make ps / ports / logs / migrate\n\n'
	@printf 'Auth:\n'
	@printf '  make refresh-auth        OAuth sync + reload stale services\n'
	@printf '  make twitch-local-auth   Device-code login for localhost:8090\n'
	@printf '  make twitch-sync         Sync Twitch CLI creds into .env\n\n'
	@printf 'Quality: make check-quick | make check | test | vet | build | clipper-test | smoke | agent-smoke\n'
	@printf 'Pulse emote (A+ path): make rebuild-analytics-emote | test-pulse-emote | smoke-pulse-emote\n'
	@printf '  make pulse-emote-pick-stream | smoke-pulse-emote-gold LOGIN=... STREAM_ID=...\n'
	@printf 'Agent MCP: make mcp-setup | make mcp-verify | make codex-setup | make claude-setup | codegraph | bash scripts/mcp-preflight.sh\n'
	@printf '          make test-video | test-analytics | test-emote | test-metadata\n'
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
	@printf 'Hosted production ops (private streampulse-ops):\n'
	@printf '  Deploy uses pinned IMAGE_TAG from GHCR — not make targets here.\n'
	@printf '  Local hybrid scrape handoff:\n'
	@printf '  make local-vps-only             Stop local scraper; remote VPS owns scrape\n'

env:
	@test -f .env || bash scripts/env-synthesize.sh core .env

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

test-analytics-gold-segments: integration-up
	INTEGRATION=1 $(GO) test ./internal/analytics/... -run 'GoldVODSegmentStore|GoldVODSegmentKey|PlanGoldVOD' -count=1 -timeout 120s

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

codegraph-pulse:
	@test -x $(CODEGRAPH_PY) || $(MAKE) codegraph-install
	@test -d "$(CODEGRAPH_PULSE_REPO)" || (echo "streamclone-pulse not found at $(CODEGRAPH_PULSE_REPO)" && exit 1)
	@mkdir -p "$(dir $(CODEGRAPH_PULSE_DB))"
	PYTHONPATH=. $(CODEGRAPH_PY) tools/codegraph/codegraph_ingest.py --repo "$(CODEGRAPH_PULSE_REPO)" --db "$(CODEGRAPH_PULSE_DB)"

mcp-setup: codegraph-install codegraph
	@bash scripts/mcp-preflight.sh
	@printf '\nNext: copy .cursor/mcp.recommended.json.example → .cursor/mcp.json (gitignored)\n'
	@printf 'Codex: make codex-setup — see docs/CODEX.md\n'

mcp-verify:
	@bash scripts/mcp-preflight.sh
	@printf '\nMCP preflight OK (no codegraph rebuild). Stale graph: make codegraph | full stack: make mcp-setup\n'

context-snapshots:
	@bash scripts/context/all.sh

context-verify:
	@bash scripts/context/verify-agent-stack.sh

codex-sync-skills:
	@bash scripts/codex-sync-skills.sh

codex-setup:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/codex-setup.ps1

claude-sync-skills:
	@bash scripts/claude-sync-skills.sh

claude-sync-agents:
	@bash scripts/claude-sync-agents.sh

claude-setup:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/claude-setup.ps1

claude-mcp-json:
	@bash scripts/claude-mcp-json-write.sh --linux

vscode-copilot-setup:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/vscode-copilot-setup.ps1

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

packages-pulse-core-test:
	cd packages/pulse-core && npm test

frontend-audit:
	cd frontend && npm audit --audit-level=high

compose-config-check: env
	$(COMPOSE_CORE) config --quiet
	IMAGE_TAG=$(VERSION) docker compose --env-file $(ENV_FILE) \
		-f deploy/docker-compose.yml \
		-f deploy/docker-compose.local-tunnel.yml \
		-f deploy/docker-compose.release.yml config --quiet
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

bearhost-config-check-release bearhost-config-check-local bearhost-config-check \
bearhost-corpus-smoke bearhost-help bearhost \
bearhost-observability-up bearhost-observability-down \
grafana grafana-up bearhost-grafana bearhost-grafana-up bearhost-grafana-tunnel-start \
grafana-stop bearhost-grafana-stop bearhost-grafana-tunnel-stop \
grafana-setup bearhost-grafana-setup bearhost-observability-enable \
grafana-sync bearhost-grafana-sync grafana-watch grafana-archive-status \
grafana-watch-install grafana-watch-uninstall grafana-watch-install-cron grafana-watch-uninstall-cron \
bearhost-observability-status bearhost-grafana-tunnel bearhost-bronze-status bearhost-corpus-only \
bearhost-rsync bearhost-analytics-predeploy-gate bearhost-cron-install:
	@bash scripts/ops-stub.sh

ifeq ($(OS),Windows_NT)
local-vps-only:
	@wsl bash -lc "cd '$$(wslpath -a '$(CURDIR)')' && bash scripts/local-vps-only.sh"
else
local-vps-only:
	@bash scripts/local-vps-only.sh
endif

archive-restore-drill:
	@bash scripts/archive-restore-drill.sh

check-quick: test vet frontend-test packages-pulse-core-test compose-config-check

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
