# domain-model

Canonical domain model for the AIbank white-label digital onboarding
platform.

`schema.json` (JSON Schema Draft 7) is the **single source of truth**.
Go, TypeScript and Python types are derived from it by the hand-rolled
generators in `codegen/` and committed to `generated/` so downstream
consumers do not need the codegen toolchain.

---

## Layout

```
packages/domain-model/
├── schema.json                       # canonical source of truth
├── Makefile                          # generate / validate / test / clean
├── codegen/
│   ├── _common.py                    # shared helpers (no third-party deps)
│   ├── generate_go.py                # → generated/go/types.go
│   ├── generate_ts.py                # → generated/ts/types.ts
│   ├── generate_python.py            # → generated/python/aibank_domain/types.py
│   └── test_generators.py            # pytest suite
└── generated/
    ├── go/
    │   ├── go.mod                    # importable Go module
    │   └── types.go                  # Code generated. DO NOT EDIT.
    ├── ts/
    │   └── types.ts                  # Code generated. DO NOT EDIT.
    └── python/
        ├── pyproject.toml            # installable as `aibank-domain-model`
        └── aibank_domain/
            ├── __init__.py
            └── types.py              # Code generated. DO NOT EDIT.
```

---

## Codegen pipeline

The generators are intentionally hand-rolled in pure Python (no
`datamodel-code-generator`, no `quicktype`, no `json-schema-to-typescript`).
This keeps the build hermetic — only Python 3.13 and a stock Go install
are required.

| Language    | Output                                      | Mapping highlights |
|-------------|---------------------------------------------|--------------------|
| Go          | `generated/go/types.go`                     | `integer` → `int64`, `date-time` → `time.Time`, enums → typed string + const block, optional → `*T` with `omitempty` |
| TypeScript  | `generated/ts/types.ts`                     | `interface` per type, enums → string-literal union + `as const` value array |
| Python      | `generated/python/aibank_domain/types.py`   | Pydantic v2 `BaseModel`, enums → `str, Enum`, optional → `T \| None = None` |

### Determinism

`make generate` is byte-stable: rerunning it with no schema changes
produces an identical file. The codegen test suite asserts this.

```bash
make generate
make test          # 8 tests including determinism + go vet + py import
make clean
```

### Adding a new type

1. Add the entry to `schema.json` under `$defs`. Use `additionalProperties: false`
   for closed objects.
2. Bump the version in the schema `$id` URI per the rules below.
3. Run `make generate`. Commit both `schema.json` and the regenerated
   files in `generated/`.
4. Add the new type name to `REQUIRED_TYPES` in
   `codegen/test_generators.py` if it is part of the canonical entity set.

### Drift policy

CI must run `make generate` and fail the build if `git diff --quiet` is
not clean. This guarantees that committed generated files match the
schema.

---

## Versioning

The schema version is encoded in the `$id`:

```
https://aibank.io/schemas/domain-model/v1.0.0/schema.json
```

* **Backward-compatible (minor bump, no ADR):** new optional field, new
  enum value, new `$defs` entry, relaxed pattern.
* **Breaking (major bump, ADR required):** removed/renamed field, new
  `required` constraint, tightened enum/pattern, type change, removed
  `$defs` entry. Must come with a migration script in
  `tools/migrations/` and N-1 API support for ≥6 months.

---

## Domain rules (non-negotiable)

* Money: `integer` (kopecks), never `number`/`float`.
* Dates: `string`, `format: date-time`, UTC ISO 8601.
* Enums: `snake_case`.
* No cascade delete; `archived_at` instead.
* Multi-tenant: every entity carries `tenant_id`; cross-tenant references are
  forbidden.
* `AuditEvent` is append-only.

---

## Owner

Principal Architect. Breaking changes require a PR with an ADR.
