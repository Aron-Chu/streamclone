GO ?= go
GO_DOCKER_IMAGE ?= golang:1.25-alpine
VERSION ?= $(shell tr -d '[:space:]' < VERSION 2>/dev/null || echo latest)
ENV_FILE ?= .env
COMPOSE_FEATURE_PROFILES ?= $(shell bash -c 'source scripts/lib/env.sh 2>/dev/null; env_feature_compose_profiles "$(ENV_FILE)"' | awk '{for (i=1;i<=NF;i++) printf " --profile %s", $$i}')
COMPOSE_CORE ?= docker compose --env-file $(ENV_FILE) -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml$(COMPOSE_FEATURE_PROFILES)
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

ENV_RELOAD_SERVICES ?= chat metadata emote frontend

.PHONY: help env up app stop down down-clean nuke restart rebuild \
	refresh-auth reload-env reload-env-if-stale ensure-oauth ensure-frontend-config \
	ps ports migrate logs \
	test vet build tidy integration-up integration-down integration-test \
	test-video test-emote test-metadata \
	go-test-docker go-vet-docker go-build-docker \
	twitch twitch-debug twitch-sync twitch-local-auth clipper-refresh-token \
	clipper-test codegraph-install codegraph codegraph-full codegraph-smoke codegraph-incremental codegraph-mcp mcp-setup codex-setup codex-sync-skills claude-setup claude-sync-skills claude-sync-agents \
	context-snapshots context-verify \
	docs-screenshots docs-media frontend-build frontend-test frontend-audit \
	frontend-restart frontend-refresh frontend-logs compose-config-check \
	check check-quick product-boundary-preflight product-boundary-strict \
	bootstrap setup validate-env security-scan smoke smoke-ui install-hooks \
	preflight-deps start stop-user ensure-localhost agent-smoke coverage-report \
	laptopworker-status laptopworker-smoke laptopworker-update laptopworker-boot-check laptopworker-setup laptopworker-setup-verify

help:
	@printf 'Streamclone — common targets\n\n'
	@printf 'Stack:\n'
	@printf '  make up / app        Start core stack\n'
	@printf '  make stop / down     Stop compose (keep data)\n'
	@printf '  make down-clean      Stop + remove pg/minio volumes\n'
	@printf '  make nuke            Full teardown: compose (all profiles), setup-control, orphans\n'
	@printf '  make restart         stop + up\n'
	@printf '  make ps / ports / logs / migrate\n\n'
	@printf 'Auth:\n'
	@printf '  make refresh-auth        OAuth sync + reload stale services\n'
	@printf '  make twitch-local-auth   Device-code login for localhost:8090\n'
	@printf '  make twitch-sync         Sync Twitch CLI creds into .env\n\n'
	@printf 'Quality: make check-quick | make check | test | vet | build | clipper-test | smoke | agent-smoke\n'
	@printf 'Agent MCP: make mcp-setup | make mcp-verify | make codex-setup | make claude-setup | codegraph | bash scripts/mcp-preflight.sh\n'
	@printf '          make test-video | test-emote | test-metadata\n'
	@printf '          make frontend-test | frontend-audit | compose-config-check\n'
	@printf '          make frontend-refresh  Build + migrate + restart frontend/chat\n'
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

env:
	@test -f .env || bash scripts/env-synthesize.sh core .env

up app: env ensure-oauth
	$(COMPOSE_CORE) up -d --build --remove-orphans
	@$(MAKE) reload-env-if-stale
	@$(MAKE) ensure-localhost PORTS=8090

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
rebuild: stop up

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
		$(COMPOSE_CORE) up -d --no-deps --force-recreate chat metadata emote local-proxy; \
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
	APP_DOMAIN=streamclone.example.invalid ACME_EMAIL=security@example.invalid docker compose --env-file $(ENV_FILE) \
		-f deploy/docker-compose.yml \
		-f deploy/docker-compose.prod.yml config --quiet

check-quick: test vet frontend-test compose-config-check product-boundary-preflight

product-boundary-preflight:
	@bash scripts/check-product-boundary.sh --preflight

product-boundary-strict:
	@STREAMCLONE_BOUNDARY_STRICT=1 bash scripts/check-product-boundary.sh --strict

check: security-scan frontend-build frontend-audit frontend-test clipper-test test vet build compose-config-check

frontend-restart: env
	$(COMPOSE_CORE) build frontend
	$(COMPOSE_CORE) up -d --no-deps --force-recreate frontend local-proxy
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-setup-control.ps1
	@$(MAKE) ensure-localhost PORTS=8090

# Rebuild frontend assets, apply DB migrations, and recreate frontend + chat.
frontend-refresh: env frontend-build
	@echo "Applying migrations..."
	$(COMPOSE_CORE) run --rm migrate
	@echo "Rebuilding frontend and chat..."
	$(COMPOSE_CORE) build frontend chat
	$(COMPOSE_CORE) up -d --no-deps --force-recreate frontend chat local-proxy
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
