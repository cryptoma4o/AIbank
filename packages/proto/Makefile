# Makefile for packages/proto — canonical gRPC contract codegen.
#
# Per ADR-0004, every codegen step is also wrapped as an Nx target so it
# participates in affected-detection. This Makefile mirrors the Nx targets and
# is the source of truth invoked by both Nx executors (`nx run proto:lint`)
# and developers running plain `make lint` from this directory.
#
# All targets are silent-failing when `buf` is not installed locally — codegen
# is enforced in CI via bufbuild/buf-action, so a developer without buf can
# still run unit tests and linters that don't depend on regenerated stubs.

PROTO_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))

.PHONY: lint breaking generate clean verify help

help:
	@echo "Targets:"
	@echo "  lint      - run buf lint over .proto files"
	@echo "  breaking  - check breaking changes against main (skipped if no main ref)"
	@echo "  generate  - regenerate Go/TS/Python stubs into generated/"
	@echo "  clean     - remove generated/ artifacts"
	@echo "  verify    - generate and assert no drift (CI guard)"

# `buf lint` — DEFAULT ruleset minus the few exceptions in buf.yaml.
# Each recipe is wrapped in a single shell invocation (`set -e ; ...`) so the
# install-guard and the actual buf invocation share one execution context;
# without that, `make` runs every recipe line in its own subshell and the
# guard cannot short-circuit subsequent commands.
lint:
	@set -e ; \
	if ! command -v buf >/dev/null 2>&1; then \
		echo "buf not installed; codegen runs in CI (bufbuild/buf-action)" ; \
		exit 0 ; \
	fi ; \
	cd $(PROTO_DIR) && buf lint

# `buf breaking` — compares against main on the local git remote. If no main
# ref is reachable (e.g. shallow CI clones, fresh local clone) we skip silently
# rather than fail; the authoritative breaking check runs in CI where the full
# history is fetched.
breaking:
	@set -e ; \
	if ! command -v buf >/dev/null 2>&1; then \
		echo "buf not installed; breaking check runs in CI" ; \
		exit 0 ; \
	fi ; \
	if ! git -C $(PROTO_DIR) rev-parse --verify main >/dev/null 2>&1 \
	   && ! git -C $(PROTO_DIR) rev-parse --verify origin/main >/dev/null 2>&1; then \
		echo "no main ref available; skipping breaking check" ; \
		exit 0 ; \
	fi ; \
	cd $(PROTO_DIR) && buf breaking --against '.git#branch=main,subdir=packages/proto'

# Full regeneration. Output paths are deterministic (see buf.gen.yaml).
generate:
	@set -e ; \
	if ! command -v buf >/dev/null 2>&1; then \
		echo "buf not installed; cannot generate locally — run in CI" ; \
		exit 1 ; \
	fi ; \
	cd $(PROTO_DIR) && buf generate

clean:
	rm -rf $(PROTO_DIR)/generated/go/aibank \
	       $(PROTO_DIR)/generated/python/aibank_proto/aibank \
	       $(PROTO_DIR)/generated/ts/aibank
	@echo "cleaned generated/ artifacts (kept go.mod/pyproject.toml/package.json scaffolding)"

# CI guard: if the committed generated/ tree disagrees with what `buf generate`
# would produce, fail with a clear pointer to the drift.
verify: generate
	@if ! git -C $(PROTO_DIR) diff --exit-code -- generated/ >/dev/null 2>&1; then \
		echo "ERROR: generated/ drifted from .proto sources."; \
		echo "Run 'make generate' in packages/proto and commit the result."; \
		git -C $(PROTO_DIR) --no-pager diff --stat -- generated/; \
		exit 1; \
	fi
	@echo "generated/ is in sync with .proto sources"
