# domain-model

Canonical domain model for the AIbank white-label digital onboarding platform.

`schema.json` is the **single source of truth**. All service contracts, API types, and database schemas are derived from it. Changes to entity structure must go through this file first.

---

## Schema overview

The schema is JSON Schema Draft 7 (`$schema: http://json-schema.org/draft-07/schema#`). All entity definitions live under `$defs`. The root object (`AIbankDomain`) acts as a namespace; code generators use it as the package/module name.

### Core entities

| Entity | ID prefix | Description |
|---|---|---|
| `Tenant` | `tnt_` | A bank using the platform. Root multi-tenant entity. |
| `Application` | `app_` | One onboarding application from draft to account opening. |
| `Person` | `per_` | Any natural person: applicant, director, founder, UBO. |
| `LegalEntity` | `le_` | Russian legal entity (ООО, АО, etc.) from EGRUL data. |
| `UBOGraph` | — | Computed beneficial ownership graph for a legal entity. |
| `Document` | `doc_` | A document uploaded by the applicant or from an external source. |
| `RiskAssessment` | — | ML-based risk score with SHAP explainability and screening results. |
| `Decision` | — | Final underwriting decision (auto or manual). |
| `Account` | `acc_` | Bank account opened after an approved application. |
| `AuditEvent` | — | Immutable, hash-linked audit log entry. |

### Value objects

| Object | Description |
|---|---|
| `Address` | Postal address with raw text and structured decomposition. |
| `UBOGraphNode` | A node (person or legal entity) in the ownership graph. |
| `UBOGraphEdge` | A directed ownership or control edge between two nodes. |
| `Timestamp` | UTC ISO 8601 date-time string. |
| `UlidId` | Typed ULID with entity-type prefix. |

---

## ID format

Every entity uses a **prefixed ULID** as its primary identifier:

```
{prefix}_{ULID}
```

Pattern: `^(tnt|app|per|le|doc|acc)_[0-9A-Z]{26}$`

Examples:
- `tnt_01HQ8X4Z9V6XYZK1234567890` — Tenant
- `app_01HQ8X4Z9V6XYZK1234567891` — Application
- `per_01HQ8X4Z9V6XYZK1234567892` — Person
- `le_01HQ8X4Z9V6XYZK1234567893`  — LegalEntity
- `doc_01HQ8X4Z9V6XYZK1234567894` — Document
- `acc_01HQ8X4Z9V6XYZK1234567895` — Account

ULID is used instead of UUID because it sorts chronologically, is URL-safe, and has a fixed length.

Entities without a typed prefix (`UBOGraph`, `RiskAssessment`, `Decision`, `AuditEvent`) use plain ULIDs or system-generated IDs appropriate to the storage backend.

---

## Design rules (non-negotiable)

- **All monetary values** are `integer` (kopecks). Never `number` or `float`.
- **All dates** are `string` with `format: date-time` (UTC ISO 8601). Local time is a UI concern only.
- **All status enums** use `snake_case`.
- **No cascade delete.** Entities are never physically deleted; mark `archived_at` and keep for at least 5 years.
- **Multi-tenant isolation.** Every entity carries a `tenant_id`. Cross-tenant references are forbidden.
- **AuditEvent is append-only.** Records form a hash-linked chain via `previous_hash` (SHA-256). Mutation is not permitted.

---

## Codegen pipeline

Generated code lives under `generated/` and is committed to the repository so downstream services have no build dependency on the toolchain.

### Prerequisites

| Language | Tool | Install |
|---|---|---|
| Go | `go-jsonschema` | `go install github.com/atombender/go-jsonschema@latest` |
| TypeScript | `json-schema-to-typescript` | `npm install -g json-schema-to-typescript` |
| Python | `datamodel-codegen` | `pip install datamodel-code-generator` |

### Running codegen

```bash
# Regenerate all languages
make generate

# Regenerate one language
make generate-go
make generate-ts
make generate-python

# Remove all generated files
make clean
```

Generated files:

```
generated/
  go/domain.go            # package domain
  typescript/domain.ts    # TypeScript interfaces
  python/domain.py        # Pydantic models (AIbankDomain)
```

### When to regenerate

Regenerate and commit whenever `schema.json` changes. CI should verify that the committed generated files match a fresh `make generate` run (diff check).

---

## Versioning

The schema version follows [Semantic Versioning 2.0](https://semver.org/). The current version is embedded in the `$id` URI:

```
https://aibank.io/schemas/domain-model/v1.0.0/schema.json
```

### What counts as backward-compatible (minor bump)

- Adding an optional field with no required constraint
- Adding a new enum value that does not break existing data
- Relaxing a constraint (e.g. widening a pattern)
- Adding a new `$defs` entry (new entity)

Procedure: bump minor version in `$id`, update `CHANGELOG.md`, regenerate.

### What counts as breaking (major bump + ADR required)

- Renaming or removing a field
- Adding a `required` constraint to an existing optional field
- Tightening a pattern or enum (removing valid values)
- Changing a field type
- Removing a `$defs` entry

Procedure:
1. Write an Architecture Decision Record in `docs/adr/` explaining the reason and migration path.
2. Bump the major version in `$id`.
3. Update `CHANGELOG.md`.
4. Provide a data migration script in `tools/migrations/`.
5. Keep N-1 API version live for at least 6 months.

---

## Entity relationship summary

```
Tenant 1──n Application
Application 1──1 Person (applicant)
Application 0..1──1 LegalEntity
Application 1──n Document
Application 1──1 RiskAssessment
Application 1──1 Decision
Application 1──n Account

LegalEntity 1──1 UBOGraph
LegalEntity n──n Person (officers, founders)

AuditEvent ──> any entity (via entity_type + entity_id)
```

---

**Owner:** Principal Architect. Breaking changes require a PR with an ADR. Reviews from the compliance lead are mandatory for changes to `AuditEvent`, `RiskAssessment`, and `Decision`.
