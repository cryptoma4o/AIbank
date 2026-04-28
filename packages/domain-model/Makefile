# Makefile for packages/domain-model
#
# Targets:
#   make generate   — regenerate Go, TypeScript, and Python types from schema.json
#   make validate   — verify schema.json parses as JSON
#   make test       — run the codegen test suite
#   make clean      — remove all generated files

.PHONY: generate generate-go generate-ts generate-python validate test clean help

PYTHON ?= python3.13

help:
	@echo "Targets:"
	@echo "  generate         regenerate all language bindings"
	@echo "  generate-go      regenerate Go types only"
	@echo "  generate-ts      regenerate TypeScript types only"
	@echo "  generate-python  regenerate Python (Pydantic) types only"
	@echo "  validate         check schema.json is well-formed"
	@echo "  test             run pytest over codegen/"
	@echo "  clean            remove all generated/ artefacts"

generate: generate-go generate-ts generate-python

generate-go:
	$(PYTHON) -m codegen.generate_go

generate-ts:
	$(PYTHON) -m codegen.generate_ts

generate-python:
	$(PYTHON) -m codegen.generate_python

validate:
	@$(PYTHON) -c "import json,sys; json.load(open('schema.json')); print('schema.json: OK')"

test:
	$(PYTHON) -m pytest codegen/ -v

clean:
	@find generated -type f ! -name '.gitignore' -delete
	@echo "cleaned generated/"
