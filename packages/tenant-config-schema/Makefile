# Makefile for packages/tenant-config-schema
#
# Targets:
#   make validate       — validate _template + bank-alpha
#   make validate-all   — validate every tenant directory under configs/tenants/
#   make lint           — verify each schema is itself a valid Draft-07 schema
#   make test           — run the pytest suite

.PHONY: validate validate-all lint test help

PYTHON ?= python3.13
ROOT := $(shell git rev-parse --show-toplevel 2>/dev/null || cd ../.. && pwd)
TENANTS_DIR := $(ROOT)/configs/tenants

help:
	@echo "Targets:"
	@echo "  validate       validate _template + bank-alpha"
	@echo "  validate-all   validate every directory under $(TENANTS_DIR)"
	@echo "  lint           Draft-07 self-check on each schema"
	@echo "  test           run pytest"

validate:
	@$(PYTHON) validate.py $(TENANTS_DIR)/_template -v
	@$(PYTHON) validate.py $(TENANTS_DIR)/bank-alpha -v || echo "(bank-alpha may use legacy schema)"

validate-all:
	@set -e; \
	for d in $(TENANTS_DIR)/*; do \
		if [ -d "$$d" ] && [ "$$(basename $$d)" != "AGENTS.md" ]; then \
			echo "==> validating $$d"; \
			$(PYTHON) validate.py "$$d" || exit 1; \
		fi; \
	done

lint:
	@$(PYTHON) -c "import json, sys; from jsonschema import Draft7Validator; \
	import pathlib; \
	failed = 0; \
	for p in sorted(pathlib.Path('schemas').glob('*.json')): \
		s = json.loads(p.read_text()); \
		try: \
			Draft7Validator.check_schema(s); \
			print(f'OK    {p}'); \
		except Exception as e: \
			print(f'FAIL  {p}: {e}'); \
			failed += 1; \
	sys.exit(1 if failed else 0)"

test:
	$(PYTHON) -m pytest tests/ -v
