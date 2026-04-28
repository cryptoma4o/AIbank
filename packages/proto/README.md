# AIbank Platform — Canonical gRPC Proto Contracts

This package is the **single source of truth** for all inter-service gRPC
contracts in the AIbank platform. Per ADR-0006 (ABS-adapter versioning) the
canonical proto tree drives both the in-platform gRPC contracts and the
boundary contract between `abs-connector` and the per-bank ABS adapters.

## File contracts

| File | Package | Service(s) | Purpose |
|------|---------|-----------|---------|
| `tenant.proto` | `aibank.v1` | `TenantService` | Tenant lifecycle (`GetTenant`, `ListTenants`, `GetTenantConfig`). |
| `identity.proto` | `aibank.v1` | `IdentityService` | Authentication (`ValidateToken`, `SendOTP`, `VerifyOTP`). |
| `document.proto` | `aibank.v1` | `DocumentService` | Applicant-document upload/verification + OCR field readback. |
| `onboarding.proto` | `aibank.v1` | `OnboardingService` | The onboarding state machine (`CreateApplication` … `SubmitApplication`). |
| `risk.proto` | `aibank.v1` | `RiskService` | Rule-engine scoring + assessment readback. |
| `audit.proto` | `aibank.v1` | `AuditService` | Append-only audit log (`LogEvent`, `ListEvents`). |

The flat layout (one `<context>.proto` at the root) is the **current** v1
contract. Once a major bump is required, new files are added next to the
existing ones with a `_v2` suffix (or under `<context>/v2/`); the v1 files
remain untouched until the N-2 deprecation window closes (see ADR-0006 §4).

## Codegen pipeline

`make generate` produces three language clients from the same `.proto` set:

```
packages/proto/
├── buf.yaml            # module + lint + breaking config (v2 workspace)
├── buf.gen.yaml        # plugin pipeline (Go, ts-proto, Python + gRPC)
├── Makefile            # lint / breaking / generate / clean / verify
├── *.proto             # 6 canonical contracts (DO NOT touch under codegen)
├── codegen/
│   ├── conftest.py
│   ├── test_protos.py        # pytest sanity + buf lint runner
│   └── sample-go-import.go   # smoke-import of generated Go module
└── generated/
    ├── go/             # buf.build/protocolbuffers/go + grpc/go output
    │   ├── go.mod      # module github.com/aibank/platform/packages/proto/generated/go
    │   └── doc.go      # placeholder package decl until *.pb.go land
    ├── ts/             # ts-proto output
    │   ├── package.json   # @aibank/proto
    │   ├── tsconfig.json
    │   └── index.ts       # barrel re-export, populated by buf generate
    └── python/
        ├── pyproject.toml
        └── aibank_proto/
            └── __init__.py
```

Plugins are pulled from the public `buf.build/<owner>/<plugin>` remote
registry — neither developers nor CI need a local `protoc` install. The only
required binary is `buf` itself; CI uses `bufbuild/buf-action`, locally:

```bash
brew install bufbuild/buf/buf       # macOS
# or
go install github.com/bufbuild/buf/cmd/buf@latest
```

### Make targets

| Target | What it does |
|--------|---------------|
| `make lint` | `buf lint` (DEFAULT minus `USE_FIELD_PRESENCE`, `ENUM_VALUE_PREFIX`). |
| `make breaking` | `buf breaking --against '.git#branch=main,subdir=packages/proto'` (silently skipped if no `main` ref). |
| `make generate` | `buf generate` — writes Go + TS + Python under `generated/`. |
| `make clean` | Removes regeneratable artifacts under `generated/` (keeps the committed `go.mod`, `pyproject.toml`, `package.json` scaffolding). |
| `make verify` | Generate, then `git diff --exit-code generated/`. CI uses this as the drift guard. |

Per ADR-0004, every Make target also has a corresponding Nx target so that
`nx affected -t generate,lint` reaches into this package whenever a `.proto`
file changes.

## Adding a new RPC

1. Edit the relevant `.proto` file (e.g. `onboarding.proto`).
2. Run `make lint` — fix any warnings.
3. Run `make breaking` against `main` to confirm the change is backwards
   compatible. Adding a field or RPC is always safe; renaming or
   re-numbering is not.
4. Run `make generate`.
5. Commit **both** the `.proto` source and every file that changed under
   `generated/`. The CI drift check (`make verify`) enforces this.

## Backwards-compat policy (ADR-0006)

- **Field-level**: never reuse a field number. Adding optional fields is
  always safe (proto3 forward-compat).
- **Service-level**: never remove or rename an RPC inside a major version.
  Add a new RPC alongside, deprecate the old one for one quarterly release.
- **Major versions**: a breaking change requires a new file (e.g.
  `onboarding_v2.proto` with `package aibank.onboarding.v2`). Both v1 and
  v2 are served simultaneously by the platform until v(N-2) reaches its
  end-of-life — that is, **three generations of the canonical proto live
  side-by-side** at any given time.
- **Per-tenant pinning**: each bank-tenant declares its desired canonical
  version in `configs/tenants/<bank>/integrations/abs.yaml`; the connector
  validates compatibility against the adapter's `Capabilities()` RPC at
  startup.

## Sample integration

`codegen/sample-go-import.go` is a build-tag-gated smoke test that imports
the generated Go module. Wiring `abs-connector` to actually use the
generated client surface is a Phase 2 migration item (see ADR-0006 §
Implementation Notes) — for now we only verify the imports link.

```bash
# Smoke-build the sample (gated behind a build tag so it does not pollute
# regular `go build ./...`):
go build -tags=protocodegen_smoke ./packages/proto/codegen/...
```
