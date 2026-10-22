# pro_manage developer entry points. `make help` lists them.
SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

COMPOSE_DIR := deploy/compose
COMPOSE     := docker compose --env-file $(COMPOSE_DIR)/.env -f $(COMPOSE_DIR)/docker-compose.dev.yml
COMPOSE_CI  := docker compose --env-file $(COMPOSE_DIR)/.env.ci -f $(COMPOSE_DIR)/docker-compose.dev.yml --profile stub
MDEMG       := docker compose --env-file $(COMPOSE_DIR)/.env -f $(COMPOSE_DIR)/docker-compose.mdemg.yml
GO_DIR      := services/pm-core
PY_DIR      := services/pm-sidecar
FE_DIR      := apps/pm-frontend
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

UXTS_PY     := uv run -q --no-project --with requests --with jsonpath-ng python
UATS_RUNNER := $(UXTS_PY) docs/tests/uats/runners/uats_runner.py
# Service URLs the UATS specs default to; override to point at another stack.
export PM_CORE_URL     ?= http://127.0.0.1:8080
export PM_WORKERS_URL  ?= http://127.0.0.1:8081
export PM_SIDECAR_URL  ?= http://127.0.0.1:7070
export PM_FRONTEND_URL ?= http://127.0.0.1:3100
export STUB_MDEMG_URL  ?= http://127.0.0.1:9997

.PHONY: help env dev dev-stub dev-down dev-logs mdemg-up mdemg-down mdemg-logs \
        build test lint smoke smoke-stub ci-local hooks \
        go-build go-test go-lint py-sync py-test py-lint fe-install fe-typecheck fe-build \
        proto proto-lint proto-breaking \
        uxts test-uats test-uats-stub test-uats-live test-uxts-hashes uxts-add-hashes \
        test-unts unts-update verify-inventories check-matrix

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

env: ## Create deploy/compose/.env from the example (never overwrites)
	@test -f $(COMPOSE_DIR)/.env || { cp $(COMPOSE_DIR)/.env.example $(COMPOSE_DIR)/.env; \
	  sed -i '' "s/^MDEMG_API_KEY=$$/MDEMG_API_KEY=$$(openssl rand -hex 24)/" $(COMPOSE_DIR)/.env; \
	  sed -i '' "s/^MDEMG_J17_TICKET_SECRET=$$/MDEMG_J17_TICKET_SECRET=$$(openssl rand -hex 32)/" $(COMPOSE_DIR)/.env; \
	  echo "wrote $(COMPOSE_DIR)/.env (edit the change-me-dev passwords if you like)"; }

# ---------------------------------------------------------------- dev stacks

dev: env ## Bring up the dev stack against the dedicated MDEMG stack (make mdemg-up first)
	$(COMPOSE) up -d --build
	@$(MAKE) --no-print-directory smoke

dev-stub: env ## Bring up the dev stack with the stub MDEMG (what PR CI runs)
	MDEMG_BASE_URL=http://stub-mdemg:9999 $(COMPOSE) --profile stub up -d --build
	@$(MAKE) --no-print-directory smoke-stub

dev-down: ## Stop the dev stack (keeps volumes; add V=1 to drop them)
	$(COMPOSE) --profile stub down $(if $(V),-v,)

dev-logs: ## Tail dev stack logs
	$(COMPOSE) --profile stub logs -f --tail=100

mdemg-up: env ## Bring up pro_manage's dedicated MDEMG stack (pinned v0.11.0-beta.3)
	$(MDEMG) up -d
	@echo "MDEMG at http://127.0.0.1:$${PM_MDEMG_PORT:-9998} (X-API-Key from $(COMPOSE_DIR)/.env)"

mdemg-down: ## Stop the dedicated MDEMG stack (keeps volumes; V=1 drops them)
	$(MDEMG) down $(if $(V),-v,)

mdemg-logs: ## Tail the dedicated MDEMG stack logs
	$(MDEMG) logs -f --tail=100

smoke: ## Probe every service in the running dev stack
	@set -a; source $(COMPOSE_DIR)/.env; set +a; scripts/smoke.sh

smoke-stub: ## Probe every service including the stub MDEMG
	@set -a; source $(COMPOSE_DIR)/.env; set +a; scripts/smoke.sh --stub

# ---------------------------------------------------------------- build/test

build: go-build fe-build ## Build everything that builds locally

test: go-test py-test fe-typecheck ## Run all unit tests and type checks

lint: go-lint py-lint proto-lint ## Run all linters

go-build: ## Build the Go binaries
	cd $(GO_DIR) && go build -ldflags "-X github.com/reh3376/pro-manage/services/pm-core/internal/config.Version=$(VERSION)" ./...

go-test: ## Go unit tests (race detector on)
	cd $(GO_DIR) && go test -race -count=1 ./...

go-lint: ## go vet (golangci-lint arrives with the first real service code)
	cd $(GO_DIR) && go vet ./...

py-sync: ## Install the sidecar environment with uv
	cd $(PY_DIR) && uv sync

py-test: py-sync ## Sidecar unit tests
	cd $(PY_DIR) && uv run pytest -q

py-lint: py-sync ## Sidecar lint and type check
	cd $(PY_DIR) && uv run ruff check . && uv run mypy src

fe-install: ## Install frontend dependencies
	cd $(FE_DIR) && pnpm install --frozen-lockfile

fe-typecheck: fe-install ## Frontend type check
	cd $(FE_DIR) && pnpm typecheck

fe-build: fe-install ## Frontend production build
	cd $(FE_DIR) && pnpm build

proto: proto-lint ## Generate Go and Python code from proto/
	cd proto && buf generate

proto-lint: ## Lint protos
	cd proto && buf lint

proto-breaking: ## Check protos for breaking changes against main
	cd proto && buf breaking --against '.git#branch=main,subdir=proto'

ci-local: lint test uxts ## What PR CI runs, minus the compose smoke and live UATS
	@echo "ci-local: ok"

# ---------------------------------------------------------------- UxTS (FSD §12)

uxts: test-uxts-hashes test-unts verify-inventories check-matrix ## Static UxTS gates (what the uxts CI job runs)

test-uats: ## UATS validate-all for the services against the running dev stack (set *_URL to retarget)
	@set -e; for t in pm-core pm-workers pm-sidecar pm-frontend; do \
	  echo "== uats: $$t"; $(UATS_RUNNER) validate-all --spec-dir docs/tests/uats/specs/$$t --base-url $$(python3 -c "import os;print(os.environ['PM_'+'$$t'.upper().replace('PM-','').replace('-','_')+'_URL'])"); done

test-uats-stub: ## UATS validate-all against the stub MDEMG (needs `make dev-stub` or the CI stack)
	MDEMG_API_KEY=$${MDEMG_API_KEY:-ci-stub-key} $(UATS_RUNNER) validate-all --spec-dir docs/tests/uats/specs/stub-mdemg --base-url $(STUB_MDEMG_URL)

test-uats-live: env ## UATS validate-all against the dedicated MDEMG stack (workstation only; pre-merge for adapter changes)
	@set -a; source $(COMPOSE_DIR)/.env; set +a; \
	  $(UATS_RUNNER) validate-all --spec-dir docs/tests/uats/specs/mdemg --base-url http://127.0.0.1:$${PM_MDEMG_PORT:-9998} --timeout 90

test-uxts-hashes: ## Verify config.sha256 on every UxTS spec (merge-blocking)
	$(UATS_RUNNER) verify-hashes --spec-dir docs/tests/uats/specs --pattern "**/*.uats.json"

uxts-add-hashes: ## Re-pin config.sha256 after editing specs (commit the result in the same PR)
	$(UATS_RUNNER) add-hashes --spec-dir docs/tests/uats/specs --pattern "**/*.uats.json"
	@$(MAKE) --no-print-directory unts-update

test-unts: ## UNTS: governed artifacts match the registry (merge-blocking)
	python3 scripts/unts.py verify

unts-update: ## UNTS: recompute the registry after a reviewed change
	python3 scripts/unts.py update

verify-inventories: ## Adjudication registries: every declaration has an entry (merge-blocking)
	python3 scripts/verify_inventories.py

check-matrix: ## Framework matrix currency (fails when 30+ days behind the newest spec)
	python3 scripts/check_matrix_currency.py

hooks: ## Install the pre-commit gate (inventories, UNTS, spec hashes, gofmt)
	git config core.hooksPath scripts/hooks
	chmod +x scripts/hooks/*
	@echo "hooks installed (git config core.hooksPath scripts/hooks)"
