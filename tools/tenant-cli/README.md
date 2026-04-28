# tenant-cli

Administrative CLI for AIbank `tenant-service`.  Manages tenants
(`platform.tenants`) and applies tenant configuration bundles
(`configs/tenants/<id>/`) defined under [docs/tenant-configuration.md](../../docs/tenant-configuration.md).

## Subcommands

### create — provision a new tenant

```bash
tenant-cli create \
  --id demo \
  --name "Демо банк" \
  --bik 044525974 \
  --inn 7700000000 \
  --deployment-mode saas
```

Posts to `POST /v1/tenants`.  The handler creates the platform-level
metadata row and provisions the per-tenant Postgres schema (`tnt_<id>`).
DDL migrations for that schema are applied separately via `db-migrator`:

```bash
db-migrator tenant --tenant-id demo --service tenant-service --source ...
```

### list — table or JSON listing

```bash
tenant-cli list                    # default table
tenant-cli list --status active    # client-side filter (TODO server-side)
tenant-cli list --format json
```

### get — single tenant

```bash
tenant-cli get --id demo
tenant-cli get --id demo --format json
```

### config-validate — JSON-Schema check

Walks `configs/tenants/<id>/` and validates each YAML against
`packages/tenant-config-schema/schemas/*.json`.  Internally shells out to
`packages/tenant-config-schema/validate.py` so CI and developer machines
agree.

```bash
tenant-cli config-validate --path configs/tenants/demo-bank
tenant-cli config-validate --path configs/tenants/demo-bank --verbose
```

Requires `python3` with `pyyaml` and `jsonschema>=4`:

```bash
pip install pyyaml 'jsonschema>=4'
```

### config-apply — upload to tenant-service

Reads `tenant.yaml` from `--path`, gzip-compresses, and PUTs to
`/v1/tenants/{id}/config`.  Schema version defaults to `1.0`.

```bash
tenant-cli config-apply --id demo --path configs/tenants/demo-bank
```

> Currently uploads only `tenant.yaml`.  A future enhancement (TODO) will
> pack the entire directory into a single archive once the server-side
> contract is finalized.

## Global flags

| Flag                    | Default                     | Notes                           |
|-------------------------|-----------------------------|---------------------------------|
| `--tenant-service-url`  | `http://tenant-service:8080`| Or env `TENANT_SERVICE_URL`     |

## Build & test

```bash
cd tools/tenant-cli
go mod tidy
go build ./...
go test ./...
```

## Docker

```bash
docker build -t aibank/tenant-cli:dev .
docker run --rm aibank/tenant-cli:dev list \
  --tenant-service-url http://tenant-service.aibank.svc.cluster.local:8080
```

The distroless image cannot run `config-validate` (no python).  Use the
`python:3.12-slim` image with both validate.py and the `tenant-cli` binary
mounted in if you need that path inside the cluster.

## TODO

- Server-side `--status` filter (currently filters client-side).
- `delete` / `suspend` subcommands (deferred — server-side missing).
- Direct DB-only mode for air-gapped environments
  (`tenant-cli create --direct --dsn ...`) bypassing the tenant-service API.
- Bundle the whole tenant directory into a single archive on `config-apply`.
