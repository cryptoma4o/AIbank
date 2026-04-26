# AIbank Platform — Proto Definitions

This package contains the protobuf source files that are the **source of truth**
for all inter-service gRPC contracts in the AIbank platform.

## Package structure

```
packages/proto/
├── buf.yaml            # Buf module config (lint, breaking-change rules)
├── buf.gen.yaml        # Code-generation plugin config
├── Makefile            # Developer shortcuts
├── common/v1/          # Shared scalar types (Money, Pagination, TenantContext …)
├── tenant/v1/          # Tenant lifecycle and configuration
├── application/v1/     # Onboarding application state machine
├── document/v1/        # Document upload and verification
├── audit/v1/           # Append-only, tamper-evident audit log
└── generated/
    ├── go/             # Auto-generated Go stubs (buf generate output)
    └── python/         # Auto-generated Python stubs (buf generate output)
```

All generated files under `generated/` are committed only in CI; local
development regenerates them on demand (see below).

## Prerequisites

Install [buf](https://buf.build/docs/installation):

```bash
brew install bufbuild/buf/buf          # macOS
# or
go install github.com/bufbuild/buf/cmd/buf@latest
```

For Go code generation, install the plugins:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

For Python code generation:

```bash
pip install grpcio-tools
```

## Common commands

| Command | Description |
|---------|-------------|
| `make generate` | Generate Go and Python stubs from all `.proto` files |
| `make lint` | Run buf lint (DEFAULT ruleset) |
| `make breaking` | Check for breaking changes against `main` branch |
| `make clean` | Remove all generated `*.pb.go` and `*.pb.ts` files |

## Adding a new service

1. Create a new directory: `packages/proto/<service>/v1/`
2. Add `<service>.proto` using `proto3` syntax.
3. Set the package to `aibank.<service>.v1` and the `go_package` option to
   `github.com/aibank/platform/gen/go/<service>/v1;<service>v1`.
4. Import `common/v1/common.proto` for shared types.
5. Run `make lint` — fix any warnings before opening a PR.
6. Run `make breaking` against `main` to ensure backwards compatibility.
7. Run `make generate` and commit the generated stubs together with the `.proto`
   source.

## Import conventions

- Always import via the module-relative path, e.g.
  `import "common/v1/common.proto";`
- Never use absolute or OS-level paths.
- External imports (e.g. `google/protobuf/timestamp.proto`) are resolved via the
  `buf.build/googleapis/googleapis` dependency declared in `buf.yaml`.

## Breaking-change policy

The buf `FILE` breaking-change checker is enforced in CI against the `main`
branch. The following changes are **forbidden** without a new major version:

- Removing or renaming a field
- Changing a field number
- Changing a field type
- Removing an RPC

Additive changes (new fields, new RPCs, new enum values) are always safe.
