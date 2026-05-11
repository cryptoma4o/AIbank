.PHONY: help generate validate-schema build test lint up down logs migrate-platform smoke smoke-clean bench bench-tenant bench-audit bench-llm bench-bff bench-seed context-light context-full agent-pr-check changed-services e2e-staging e2e-staging-build e2e-staging-api e2e-staging-install-timer rag-up rag-index rag-reindex rag-smoke

help:
	@echo "AIbank Makefile"
	@echo ""
	@echo "  make generate         — Generate Go/TS/Python types from domain-model JSON Schema"
	@echo "  make validate-schema  — Validate all tenant configs against schema"
	@echo "  make build            — Build all Docker images"
	@echo "  make test             — Run all tests"
	@echo "  make lint             — Run all linters"
	@echo "  make up               — Start full stack (docker-compose)"
	@echo "  make down             — Stop full stack"
	@echo "  make logs             — Tail all service logs"
	@echo "  make migrate-platform — Apply tenant-service platform migrations (one-shot job)"
	@echo "  make smoke            — Run end-to-end happy-path smoke test"
	@echo "  make smoke-clean      — Tear down stack and remove smoke state"
	@echo "  make eval-agents      — Run AI agent eval harness (requires running stack)"
	@echo ""
	@echo "  make bench            — Run all k6 perf-benchmarks + compare to baselines"
	@echo "  make bench-seed       — Pre-seed test data for benchmarks (5 tenants, 250 audit events)"
	@echo "  make bench-tenant     — k6 perf-bench для tenant-service"
	@echo "  make bench-audit      — k6 perf-bench для audit-service"
	@echo "  make bench-llm        — k6 perf-bench для llm-gateway (mock mode)"
	@echo "  make bench-bff        — k6 perf-bench для bff-onboarding GraphQL"
	@echo ""
	@echo "  --- Для AI-агентов / Claude Code -----------------"
	@echo "  make context-light    — Компактная карта проекта (для агентов, ~200 токенов)"
	@echo "  make context-full     — Расширенный обзор + LOC статистика"
	@echo "  make changed-services — Какие services/ai/apps затронуты в текущем git diff"
	@echo "  make agent-pr-check   — Pre-PR проверки: lint + validate-schema + test (fast)"

generate:
	@echo "→ Validating domain-model schema..."
	cd packages/domain-model && python3 -c "import json; json.load(open('schema.json'))" && echo "  schema.json OK"
	@echo "→ Types are hand-written stubs (run 'make generate-codegen' to wire up full codegen)"

validate-schema:
	@echo "→ Validating tenant configs..."
	@for f in configs/tenants/*/config.yaml; do \
		python3 packages/tenant-config-schema/validate.py "$$f"; \
	done

build:
	@echo "→ Building all Docker images..."
	docker compose build --parallel

test: test-go test-python

test-go:
	@echo "→ Testing Go packages..."
	@for dir in packages/rule-engine packages/crypto-utils packages/audit-sdk/go; do \
		[ -f "$$dir/go.mod" ] && (echo "  $$dir" && cd $$dir && go test ./... -count=1) || true; \
	done
	@echo "→ Testing Go services (short mode)..."
	@for dir in services/*/; do \
		[ -f "$${dir}go.mod" ] && (echo "  $$dir" && cd $$dir && go test ./... -short -count=1) || true; \
	done

test-python:
	@echo "→ Testing Python eval-harness..."
	cd ai/eval-harness && python -m pytest tests/ -v

lint:
	@echo "→ Go lint (golangci-lint required)..."
	@command -v golangci-lint >/dev/null && \
		for dir in packages/*/ services/*/; do \
			[ -f "$${dir}go.mod" ] && (cd $$dir && golangci-lint run ./... 2>&1 | head -20) || true; \
		done || echo "  golangci-lint not installed, skipping"
	@echo "→ Python lint (ruff required)..."
	@command -v ruff >/dev/null && ruff check ai/ packages/audit-sdk/python/ tools/data-generator/ || echo "  ruff not installed, skipping"

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f --tail=100

migrate-platform:
	docker compose --profile migrate run --rm db-migrator-platform

smoke:
	bash scripts/smoke.sh

smoke-clean:
	docker compose down -v
	rm -rf /tmp/aibank-smoke-state

eval-agents:
	@echo "→ Generating eval cases..."
	cd ai/eval-harness && python -m harness.cli generate document-intake --count 5 > /tmp/eval-document-intake.json
	cd ai/eval-harness && python -m harness.cli run /tmp/eval-document-intake.json --agent-url http://localhost:8101

# ─── k6 perf-benchmarks (см. tools/benchmarks/README.md) ─────────────────
bench:
	bash tools/benchmarks/scripts/run-all.sh

bench-seed:
	bash tools/benchmarks/scripts/seed.sh

bench-tenant:
	k6 run tools/benchmarks/k6/tenant-service-bench.js

bench-audit:
	k6 run tools/benchmarks/k6/audit-service-bench.js

bench-llm:
	k6 run tools/benchmarks/k6/llm-gateway-bench.js

bench-bff:
	k6 run tools/benchmarks/k6/bff-onboarding-bench.js

# ─── AI-агентские таргеты (для Claude Code и подобных) ─────────────────
context-light:
	@bash scripts/context-light.sh

context-full:
	@bash scripts/context-light.sh --full

changed-services:
	@bash scripts/changed-services.sh

agent-pr-check:
	@echo "→ Pre-PR проверки..."
	@$(MAKE) --no-print-directory validate-schema
	@$(MAKE) --no-print-directory lint
	@$(MAKE) --no-print-directory test-go
	@echo "✓ Готово. Если PR трогает ai/, запусти также: make eval-agents"

# ── E2E staging tests ─────────────────────────────────────────────────────
# Прогоняет 10 API-сценариев онбординга + Playwright UI-smoke против
# live-стека. Результаты — append-only в .omc/e2e-results.jsonl.
#
# Окружение:
#   E2E_BASE_URL  — http://localhost (default, для прогона на staging-сервере)
#                   или http://206.204.106.28 (для прогона с локальной машины)
#   E2E_TENANT    — demo (default)
#   E2E_SKIP_UI=1 — пропустить Playwright (быстрый API-only прогон ~1 мин)

e2e-staging-build:
	@echo "→ Сборка Docker-образа aibank-e2e-runner..."
	docker build -f tests/e2e/docker/Dockerfile.runner -t aibank-e2e-runner:latest .

e2e-staging-api:  ## Быстрый API-only прогон без Docker (15 тестов, ~2 сек)
	@E2E_BASE_URL=$${E2E_BASE_URL:-http://localhost} \
	 E2E_TENANT=$${E2E_TENANT:-demo} \
	 E2E_SKIP_UI=1 \
	 bash tests/e2e/runner/run.sh

e2e-staging: e2e-staging-build  ## Полный E2E прогон в Docker (API + UI)
	docker run --rm --network host \
	  -v $$(pwd)/.omc:/app/.omc \
	  -e E2E_BASE_URL=$${E2E_BASE_URL:-http://localhost} \
	  -e E2E_TENANT=$${E2E_TENANT:-demo} \
	  aibank-e2e-runner:latest

e2e-staging-install-timer:  ## Поставить systemd timer (hourly) на staging-сервер
	@echo "→ Копирую unit-файлы..."
	scp -i ~/.ssh/aibank_deploy infrastructure/systemd/aibank-e2e.service \
	  infrastructure/systemd/aibank-e2e.timer \
	  root@206.204.106.28:/etc/systemd/system/
	@echo "→ Активирую timer..."
	ssh -i ~/.ssh/aibank_deploy root@206.204.106.28 \
	  'systemctl daemon-reload && systemctl enable --now aibank-e2e.timer && systemctl list-timers aibank-e2e.timer'

# ── RAG: индексация банковской нормативки ────────────────────────────────
# Pipeline: скачать → распарсить → нарезать → POST /v1/index в rag-service.
# См. tools/legal-corpus/README.md

rag-up:  ## Поднять tei + rag-service (Qdrant уже должен быть UP)
	docker compose up -d tei rag-service
	@echo "→ Жду готовности (BGE-M3 cold-load ~2-3 мин)..."
	@for i in $$(seq 1 60); do \
	  if curl -fsS http://localhost:8105/healthz >/dev/null 2>&1; then \
	    echo "✓ rag-service healthy ($$i × 5s)"; exit 0; \
	  fi; sleep 5; \
	done; echo "✗ rag-service не поднялся за 5 мин"; exit 1

rag-index: rag-up  ## Скачать тексты законов + проиндексировать (один раз)
	cd tools/legal-corpus && python3 fetch.py
	cd tools/legal-corpus && RAG_SERVICE_URL=http://localhost:8105 python3 chunk_and_index.py

rag-reindex:  ## Принудительная переиндексация (force fetch + recreate)
	cd tools/legal-corpus && python3 fetch.py --force
	cd tools/legal-corpus && RAG_SERVICE_URL=http://localhost:8105 python3 chunk_and_index.py --recreate

rag-smoke:  ## E2E проверка поиска по нормативке
	@E2E_BASE_URL=http://localhost python3 -m pytest tests/e2e/api/test_rag_service.py -v
