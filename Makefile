.PHONY: help generate validate-schema build test lint up down logs

help:
	@echo "AIbank Makefile"
	@echo ""
	@echo "  make generate        — Generate Go/TS/Python types from domain-model JSON Schema"
	@echo "  make validate-schema — Validate all tenant configs against schema"
	@echo "  make build           — Build all Docker images"
	@echo "  make test            — Run all tests"
	@echo "  make lint            — Run all linters"
	@echo "  make up              — Start full stack (docker-compose)"
	@echo "  make down            — Stop full stack"
	@echo "  make logs            — Tail all service logs"
	@echo "  make eval-agents     — Run AI agent eval harness (requires running stack)"

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

eval-agents:
	@echo "→ Generating eval cases..."
	cd ai/eval-harness && python -m harness.cli generate document-intake --count 5 > /tmp/eval-document-intake.json
	cd ai/eval-harness && python -m harness.cli run /tmp/eval-document-intake.json --agent-url http://localhost:8101
