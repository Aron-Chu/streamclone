GO ?= go
ENV_FILE ?= .env
COMPOSE_CORE ?= docker compose --env-file $(ENV_FILE) -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml
COMPOSE_SCRAPER ?= $(COMPOSE_CORE) --profile scraper
COMPOSE_FULL ?= $(COMPOSE_CORE) --profile scraper --profile clipper
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

CODEGRAPH_VENV ?= .codegraph/.venv
CODEGRAPH_PY ?= $(CODEGRAPH_VENV)/bin/python
CODEGRAPH_DB ?= .codegraph/streamclone.kuzu

ENV_RELOAD_SERVICES ?= chat metadata analytics emote

.PHONY: help env up app stop down down-clean nuke restart rebuild up-scraper up-full \
	refresh-auth reload-env reload-env-if-stale ensure-oauth ensure-clipper-auth ensure-frontend-config \
	scraper-reload scraper-check scraper-preflight scraper-warm ps ports migrate logs \
	helm-kubeconfig helm-up helm-down helm-status helm-lint \
	test vet build tidy integration-up integration-down integration-test \
	twitch twitch-debug twitch-sync twitch-local-auth clipper-refresh-token \
	clipper-test clipper-restart codegraph-install codegraph codegraph-mcp \
	docs-screenshots docs-media frontend-build frontend-restart frontend-logs \
	bootstrap setup validate-env security-scan smoke smoke-ui install-hooks \
	preflight-deps start stop-user

help:
	@printf 'Streamclone — common targets\n\n'
	@printf 'Stack:\n'
	@printf '  make up / app        Start core stack\n'
	@printf '  make up-scraper      Core + TwitchTracker scraper\n'
	@printf '  make up-full         Scraper + clipper\n'
	@printf '  make stop / down     Stop compose (keep data)\n'
	@printf '  make down-clean      Stop + remove pg/minio/clipper volumes\n'
	@printf '  make nuke            down-clean + helm pulse + integration + orphans\n'
	@printf '  make restart         stop + up\n'
	@printf '  make rebuild         stop + up-full\n'
	@printf '  make ps / ports / logs / migrate\n\n'
	@printf 'Auth:\n'
	@printf '  make refresh-auth        OAuth sync + reload stale services\n'
	@printf '  make twitch-local-auth   Device-code login for localhost:8090\n'
	@printf '  make twitch-sync         Sync Twitch CLI creds into .env\n\n'
	@printf 'Helm (Emote Pulse sandbox): docs/helm-pulse.md\n'
	@printf '  make helm-kubeconfig  Link Docker Desktop kubeconfig (WSL)\n'
	@printf '  make helm-up / down / status / lint\n\n'
	@printf 'Quality: make test | vet | build | clipper-test | smoke | security-scan\n'

env:
	@test -f .env || cp .env.dev .env

up app: env ensure-oauth
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

stop down:
	@ENV_FILE=$(ENV_FILE) bash scripts/compose-down.sh

down-clean:
	@ENV_FILE=$(ENV_FILE) bash scripts/compose-down.sh --volumes

nuke:
	@ENV_FILE=$(ENV_FILE) bash scripts/nuke.sh

restart: stop up
rebuild: stop up-full

reload-env: env
	@echo "Recreating env-sensitive services ($(ENV_RELOAD_SERVICES))..."
	$(COMPOSE_CORE) up -d --no-deps --force-recreate $(ENV_RELOAD_SERVICES)

reload-env-if-stale: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/reload-env-if-stale.ps1 -EnvFile $(ENV_FILE)

ensure-oauth: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-oauth-env.ps1 -EnvFile $(ENV_FILE) || true

ensure-clipper-auth: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-clipper-auth.ps1 -EnvFile $(ENV_FILE) || true

ensure-frontend-config: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/ensure-frontend-config.ps1 -EnvFile $(ENV_FILE) || true

refresh-auth: env ensure-oauth reload-env-if-stale ensure-clipper-auth

scraper-reload: env
	$(COMPOSE_SCRAPER) up -d --no-deps --force-recreate scraper

scraper-check: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/scraper-preflight.ps1 -CheckOnly

scraper-preflight: env
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/scraper-preflight.ps1

scraper-warm:
	@$(POWERSHELL) -ExecutionPolicy Bypass -File scripts/warm-camoufox-profile.ps1

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

helm-up: helm-kubeconfig
	@values=""; \
	if [ -f "$(HELM_LOCAL_VALUES)" ]; then values="-f $(HELM_LOCAL_VALUES)"; \
	elif [ -f "$(HELM_EXAMPLE_VALUES)" ]; then values="-f $(HELM_EXAMPLE_VALUES)"; fi; \
	$(HELM) upgrade --install $(HELM_RELEASE) $(HELM_CHART) \
		-n $(HELM_NAMESPACE) --create-namespace $$values --wait

helm-down:
	-$(HELM) uninstall $(HELM_RELEASE) -n $(HELM_NAMESPACE)

helm-status:
	kubectl -n $(HELM_NAMESPACE) get pods,svc

helm-lint:
	$(HELM) lint $(HELM_CHART)

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

codegraph-install:
	python3 -m venv $(CODEGRAPH_VENV)
	$(CODEGRAPH_PY) -m pip install -r tools/codegraph/requirements.txt

codegraph:
	@test -x $(CODEGRAPH_PY) || $(MAKE) codegraph-install
	$(CODEGRAPH_PY) tools/codegraph/codegraph_ingest.py --repo . --db $(CODEGRAPH_DB)

codegraph-mcp:
	$(CODEGRAPH_PY) tools/codegraph/codegraph_mcp.py --repo "$(CURDIR)" --db "$(CURDIR)/$(CODEGRAPH_DB)"

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
		$(COMPOSE_FULL) up -d --force-recreate clipper; \
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

preflight-deps:
	@bash scripts/preflight-deps.sh --install-hints

start:
	@bash scripts/start-streamclone.sh

stop-user:
	@bash scripts/stop-streamclone.sh
