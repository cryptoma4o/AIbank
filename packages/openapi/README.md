# packages/openapi

OpenAPI 3.0.3 specs for every public HTTP API on the platform. **Specs are
written before implementation** and are the single source of truth for
the wire shape; if a service handler diverges from the spec here, that's
a bug — fix the spec first, then the handler (or vice versa, with the
spec change in the same PR).

## What lives here

| File | Service / surface | Status |
|---|---|---|
| `tenant-service.yaml` | tenant-service (`:8080`) | implemented |
| `audit-service.yaml` | audit-service (`:8081`) | implemented |
| `identity-service.yaml` | identity-service (`:8082`) | implemented |
| `document-service.yaml` | document-service (`:8083`) | implemented |
| `onboarding-orchestrator.yaml` | onboarding-orchestrator (`:8085`) | implemented |
| `risk-engine.yaml` | risk-engine (`:8086`) | implemented |
| `bff-onboarding.yaml` | BFF GraphQL gateway | placeholder — GraphQL has its own SDL, see below |
| `ext-egrul.yaml` | ЕГРЮЛ adapter (`:8090`) | implemented |

The 7 backend services in scope for this milestone are all implemented in
this directory (the BFF is a GraphQL gateway, see "Out of scope" below).

## What is NOT here

- **GraphQL schemas.** `bff-onboarding` and `bff-admin` expose a single
  `POST /graphql` endpoint; their SDL lives next to the service code
  (`services/bff-*/internal/graph/schema.graphqls`). The OpenAPI file in
  this directory is intentionally a stub that describes only the
  HTTP envelope around `/graphql` and the health endpoints — codegen for
  GraphQL goes through the gqlgen / Apollo toolchain, not openapi-generator.
- **Internal-only RPC.** Anything spoken over Temporal queues, NATS, or
  in-process buses is not an HTTP API and does not belong here.
- **AI agent contracts.** Each `ai/` agent exposes a small HTTP
  surface; once those stabilise they will land here as
  `ai-<agent-name>.yaml`. Today they are evolving too quickly.

## Spec policy

1. **Contracts first.** A new endpoint MUST land in the relevant yaml
   file in the same PR as the handler change. Reviewers should reject
   PRs that only touch the handler.
2. **No fictional fields.** Every field in a schema must map to a real
   field in the corresponding handler/domain. The 7 implemented specs
   were validated against the Go source under `services/<svc>/internal/`.
3. **Reuse via `$ref`.** Component schemas are deduplicated; new
   endpoints should reach for `#/components/schemas/...` before adding
   inline objects.
4. **All errors share one shape**, mirroring the `writeError` helper in
   each Go service:

   ```json
   { "error": { "code": "validation_failed", "message": "tenant_id is required" } }
   ```

   Every yaml here defines `ErrorResponse` with this shape.
5. **Examples are required.** Every request body and every non-200
   response must carry at least one `example`. Generators surface these
   as defaults in client SDKs and as fixtures in mocks.
6. **OpenAPI 3.0.3.** Pin the version explicitly. Do not jump to 3.1
   without an ADR — the toolchain (Spectral, openapi-generator) is still
   patchier on 3.1.

## Validation

PR CI runs (or will run; see TODO) the following on every change:

```bash
# Parse-only (lightweight check baked into CI today):
for f in packages/openapi/*.yaml; do
  python3 -c "import yaml; yaml.safe_load(open('$f'))"
done
```

Stricter linting via Spectral lives behind an Nx target:

```bash
npx -y @stoplight/spectral-cli lint packages/openapi/*.yaml
```

The spec rules are not yet checked in (`.spectral.yaml` TODO).

## Codegen

Targeted clients are generated through Nx (ADR-0004). A typical wiring:

```jsonc
// services/<svc>/project.json
{
  "targets": {
    "openapi:gen": {
      "executor": "nx:run-commands",
      "options": {
        "commands": [
          "openapi-generator-cli generate \
             -i packages/openapi/<svc>.yaml \
             -g go \
             -o packages/clients/<svc>-go"
        ]
      }
    }
  }
}
```

The `audit-sdk` is hand-written today (small surface, hot path) but its
shapes are kept in lockstep with `audit-service.yaml`.

## TODOs

- Spectral ruleset (`.spectral.yaml`) and Nx `openapi:lint` target.
- BFF GraphQL SDL contract (lives next to the service, but cross-link
  from here once it stabilises).
- `ai/agent-*` agent specs once their HTTP surfaces stabilise.
- `bff-admin.yaml`, `client-service.yaml`, `notification-service.yaml`
  follow when those services are ready for external callers.
