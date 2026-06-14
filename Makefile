GO ?= go
ENV_FILE ?= .env
COMPOSE_CORE ?= docker compose --env-file $(ENV_FILE) -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml
COMPOSE ?= $(COMPOSE_CORE)
COMPOSE_SCRAPER ?= $(COMPOSE_CORE) --profile scraper
COMPOSE_FULL ?= $(COMPOSE_CORE) --profile scraper --profile clipper
OBS_COMPOSE ?= docker compose --env-file $(ENV_FILE) -f deploy/docker-compose.yml -f deploy/docker-compose.observability.yml
PROD_COMPOSE ?= docker compose --env-file $(ENV_FILE) -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml
BASE_COMPOSE ?= docker compose --env-file $(ENV_FILE) -f deploy/docker-compose.yml
POWERSHELL ?= powershell.exe
TWITCH_SCOPES ?= chat:read chat:edit user:read:follows clips:edit
TWITCH_LOCAL_AUTH_URL ?= http://localhost:8090
TWITCH_ACTION ?= sync
TWITCH_CLIP_LOGIN ?= sodapoppin
CLIPPER_PYTHON ?= python3
PROFILE ?= core

CODEGRAPH_VENV ?= .codegraph/.venv
CODEGRAPH_PY ?= $(CODEGRAPH_VENV)/bin/python
CODEGRAPH_DB ?= .codegraph/streamclone.kuzu

# Services that read TWITCH_OAUTH_* / session env at container create time (restart is not enough).
ENV_RELOAD_SERVICES ?= chat metadata analytics emote

.PHONY: env ensure-oauth ensure-clipper-auth ensure-frontend-config refresh-auth rebuild app stop restart up down down-clean ps ports migrate logs obs-up obs-down obs-logs obs-config test vet build tidy twitch twitch-debug twitch-version twitch-configure twitch-sync twitch-token twitch-local-auth clipper-test clipper-run clipper-restart codegraph-install codegraph codegraph-mcp docs-screenshots docs-media frontend-build frontend-restart frontend-logs up-scraper up-full bootstrap setup setup-core setup-full validate-env security-scan smoke smoke-ui install-hooks reload-env reload-env-if-stale scraper-reload scraper-check scraper-preflight scraper-warm preflight-deps start stop-user

preflight-deps:
	@bash scripts/preflight-deps.sh --install-hints

start:
	@bash scripts/start-streamclone.sh

stop-user:
	@bash scripts/stop-streamclone.sh

reload-env: env
	@echo "Recreating env-sensitive services ($(ENV_RELOAD_SERVICES))..."
	$(COMPOSE_CORE) up -d --no-deps --force-recreate $(ENV_RELOAD_SERVICES)

reload-env-if-stale: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/reload-env-if-stale.ps1 -EnvFile $(ENV_FILE)

ensure-oauth: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-oauth-env.ps1 -EnvFile $(ENV_FILE) || true

# Validate/refresh clipper user token and force-recreate clipper when .env drifted (restart is not enough).
ensure-clipper-auth: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-clipper-auth.ps1 -EnvFile $(ENV_FILE) || true

ensure-frontend-config: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-frontend-config.ps1 -EnvFile $(ENV_FILE) || true

# Sync OAuth app creds, reload stale Go services, refresh clipper token when possible.
refresh-auth: env ensure-oauth reload-env-if-stale ensure-clipper-auth

env:
	@test -f .env || cp .env.dev .env

app: env ensure-oauth
	$(COMPOSE_CORE) up -d --build --remove-orphans
	@$(MAKE) reload-env-if-stale

up-scraper: env ensure-oauth
	$(COMPOSE_SCRAPER) up -d --build --remove-orphans
	@$(MAKE) reload-env-if-stale
	@if [ -z "$$SCRAPER_SKIP_PREFLIGHT" ]; then $(MAKE) scraper-preflight; fi

up-full: env ensure-oauth
	$(COMPOSE_FULL) up -d --build --remove-orphans
	@$(MAKE) reload-env-if-stale
	@$(MAKE) ensure-clipper-auth
	@if [ -z "$$SCRAPER_SKIP_PREFLIGHT" ]; then $(MAKE) scraper-preflight; fi

# Full teardown + standup with OAuth/clipper token hygiene. Prefer over bare restart after .env or auth changes.
rebuild: stop up-full

scraper-reload: env
	$(COMPOSE_SCRAPER) up -d --no-deps --force-recreate scraper

scraper-check: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/scraper-preflight.ps1 -CheckOnly

scraper-preflight: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/scraper-preflight.ps1

scraper-warm:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/warm-camoufox-profile.ps1

stop: env
	@echo "Stopping all Streamclone compose stacks..."
	-$(COMPOSE_FULL) down --remove-orphans --timeout 30
	-$(COMPOSE_CORE) down --remove-orphans --timeout 30
	-$(OBS_COMPOSE) down --remove-orphans --timeout 30
	-$(PROD_COMPOSE) down --remove-orphans --timeout 30
	-$(BASE_COMPOSE) down --remove-orphans --timeout 30
	-@docker rm -f streamclone-chat-tunnel 2>/dev/null || true
	@echo "Done. Run 'make ps' to verify nothing is still listening on app ports."

# Full teardown + standup. Use after .env changes if you prefer a clean slate over make reload-env.
restart: stop app

up: app

down: stop

down-clean: env
	@echo "Stopping stacks and removing named volumes (pg-data, minio-data, clipper-data)..."
	-$(COMPOSE_FULL) down --remove-orphans -v --timeout 30
	-$(COMPOSE_CORE) down --remove-orphans -v --timeout 30
	-$(OBS_COMPOSE) down --remove-orphans -v --timeout 30
	-$(PROD_COMPOSE) down --remove-orphans -v --timeout 30
	-$(BASE_COMPOSE) down --remove-orphans -v --timeout 30
	-@docker rm -f streamclone-chat-tunnel 2>/dev/null || true

ps: env
	@docker ps -a --filter "name=streamclone" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

ports:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/stack-ports.ps1

migrate: env
	$(COMPOSE_CORE) run --rm migrate

logs: env
	$(COMPOSE_CORE) logs -f

obs-up: env
	$(OBS_COMPOSE) up -d --build

obs-down: env
	$(OBS_COMPOSE) down

obs-logs: env
	$(OBS_COMPOSE) logs -f prometheus grafana loki promtail

obs-config:
	$(OBS_COMPOSE) config

test:
	$(GO) test ./...

integration-up:
	docker compose -f internal/integration/docker-compose.test.yml up -d

integration-down:
	docker compose -f internal/integration/docker-compose.test.yml down -v

integration-test: integration-up
	INTEGRATION=1 $(GO) test ./internal/integration/ -count=1 -timeout 120s

vet:
	$(GO) vet ./...

build:
	$(GO) build ./...

tidy:
	$(GO) mod tidy

clipper-test:
	PYTHONPATH=clipper $(CLIPPER_PYTHON) -m unittest discover -s clipper/tests

clipper-run:
	PYTHONPATH=clipper $(CLIPPER_PYTHON) -m liveclipper

codegraph-install:
	python3 -m venv $(CODEGRAPH_VENV)
	$(CODEGRAPH_PY) -m pip install -r tools/codegraph/requirements.txt

codegraph:
	@test -x $(CODEGRAPH_PY) || $(MAKE) codegraph-install
	$(CODEGRAPH_PY) tools/codegraph/codegraph_ingest.py --repo . --db $(CODEGRAPH_DB)

codegraph-mcp:
	$(CODEGRAPH_PY) tools/codegraph/codegraph_mcp.py --repo "$(CURDIR)" --db "$(CURDIR)/$(CODEGRAPH_DB)"

twitch: env
	@if [ "$(TWITCH_ACTION)" = "version" ]; then \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action version; \
	elif [ "$(TWITCH_ACTION)" = "configure" ]; then \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action configure; \
	elif [ "$(TWITCH_ACTION)" = "token" ]; then \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action token -Scopes "$(TWITCH_SCOPES)"; \
	elif [ "$(TWITCH_ACTION)" = "sync" ]; then \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action sync-env -EnvFile $(ENV_FILE); \
		$(MAKE) reload-env; \
	elif [ "$(TWITCH_ACTION)" = "local-auth" ]; then \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action sync-env -EnvFile $(ENV_FILE); \
		$(COMPOSE_FULL) up -d --no-deps --force-recreate chat metadata analytics emote local-proxy; \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action local-auth -Scopes "$(TWITCH_SCOPES)" -ChatHttp "$(TWITCH_LOCAL_AUTH_URL)"; \
	elif [ "$(TWITCH_ACTION)" = "refresh-clipper-token" ]; then \
		$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action refresh-clipper-token -EnvFile $(ENV_FILE); \
		$(COMPOSE_FULL) up -d --force-recreate clipper; \
	else \
		echo "Unsupported TWITCH_ACTION=$(TWITCH_ACTION). Use version, configure, token, sync, local-auth, or refresh-clipper-token."; \
		exit 1; \
	fi

twitch-debug: env
	@printf 'Auth debug:\n'
	@curl -fsS $(TWITCH_LOCAL_AUTH_URL)/v1/auth/debug
	@printf '\n\nClip probe (%s):\n' "$(TWITCH_CLIP_LOGIN)"
	@curl -fsS "$(TWITCH_LOCAL_AUTH_URL)/v1/channels/$(TWITCH_CLIP_LOGIN)/clips?limit=1"
	@printf '\n'

twitch-version:
	@$(MAKE) twitch TWITCH_ACTION=version

twitch-configure:
	@$(MAKE) twitch TWITCH_ACTION=configure

twitch-sync:
	@$(MAKE) twitch TWITCH_ACTION=sync

twitch-token:
	@$(MAKE) twitch TWITCH_ACTION=token

twitch-local-auth:
	@$(MAKE) twitch TWITCH_ACTION=local-auth

clipper-refresh-token:
	@$(MAKE) twitch TWITCH_ACTION=refresh-clipper-token

docs-screenshots:
	cd frontend && npx playwright install chromium && npm run screenshots:readme

docs-media:
	cd frontend && npx playwright install chromium && npm run docs:media

docs-media-analytics:
	cd frontend && npx playwright install chromium && npm run docs:media:analytics

frontend-build:
	cd frontend && npm run build

frontend-restart: env
	$(COMPOSE_CORE) build frontend
	$(COMPOSE_CORE) up -d --no-deps --force-recreate frontend local-proxy

clipper-restart: env ensure-clipper-auth
	$(COMPOSE_FULL) build clipper
	$(COMPOSE_FULL) up -d --no-deps --force-recreate clipper local-proxy

frontend-logs: env
	$(COMPOSE_CORE) logs -f frontend

bootstrap:
	@bash scripts/bootstrap.sh

setup:
	@bash scripts/setup.sh

setup-core:
	@bash scripts/setup.sh --profile core --non-interactive

setup-full:
	@bash scripts/setup.sh --profile full --non-interactive

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
