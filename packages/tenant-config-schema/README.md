# tenant-config-schema

JSON Schema definitions and a CLI validator for AIbank tenant
configuration files. Every YAML/JSON file under
`configs/tenants/<id>/` is routed to a schema in `schemas/` and
validated before deployment (CI gate per ADR-0009).

---

## Layout

```
packages/tenant-config-schema/
├── schemas/
│   ├── tenant.json         # tenant.yaml
│   ├── branding.json       # branding/theme.json + branding/content.yaml
│   ├── workflow.json       # workflows/{ip,llc,jsc}.yaml
│   ├── risk-policy.json    # risk-policy/{rules,thresholds,blocked-okveds}.yaml
│   ├── integrations.json   # integrations/{abs,external}.yaml
│   ├── ai.json             # ai/{models,prompts}.yaml
│   └── sla.json            # sla.yaml
├── validate.py             # CLI entry point
├── tests/
│   └── test_validate.py    # pytest suite
├── pyproject.toml
├── Makefile
└── README.md
```

All schemas are JSON Schema **Draft 7** (`$schema:
http://json-schema.org/draft-07/schema#`) — the most widely supported
draft across language ecosystems and the variant used by `jsonschema>=4`.

---

## File-to-schema routing

`validate.py` picks the schema based on the file's path **inside the
tenant directory**:

| Path inside tenant dir              | Schema                |
|-------------------------------------|-----------------------|
| `tenant.yaml`                       | `tenant.json`         |
| `sla.yaml`                          | `sla.json`            |
| `branding/theme.json`               | `branding.json`       |
| `branding/content.yaml`             | `branding.json`       |
| `workflows/*.yaml`                  | `workflow.json`       |
| `risk-policy/rules.yaml`            | `risk-policy.json`    |
| `risk-policy/thresholds.yaml`       | `risk-policy.json`    |
| `risk-policy/blocked-okveds.yaml`   | `risk-policy.json`    |
| `integrations/abs.yaml`             | `integrations.json`   |
| `integrations/external.yaml`        | `integrations.json`   |
| `ai/models.yaml`                    | `ai.json`             |
| `ai/prompts.yaml`                   | `ai.json`             |
| anything else                       | skipped (not routed)  |

For schemas that cover multiple files (`branding`, `risk-policy`,
`integrations`, `ai`), the schema uses `oneOf` over a set of named
variants in `definitions/`. The variant is selected by the top-level
key present in the document.

---

## Installation

```bash
python3.13 -m venv .venv
.venv/bin/pip install -e .
```

Or for testing:

```bash
.venv/bin/pip install -e '.[test]'
```

---

## Usage

```bash
# Validate one tenant directory
python validate.py configs/tenants/_template
python validate.py configs/tenants/bank-alpha -v

# Or via the installed entry point
aibank-validate-tenant configs/tenants/_template
```

Exit codes:
* `0` — every routed file validates
* `1` — first validation failure (path + JSON pointer + message printed)
* `2` — usage error or missing schema

---

## Make targets

```bash
make validate        # _template + bank-alpha
make validate-all    # every directory under configs/tenants/
make lint            # Draft-07 self-check on each schema in schemas/
make test            # pytest tests/
```

`make validate-all` is the CI gate per ADR-0009.

---

## Adding a new tenant

1. `cp -r configs/tenants/_template configs/tenants/<your-tenant>`
2. Fill in real values in `tenant.yaml`, `sla.yaml`, etc.
3. Replace placeholder Vault references with real ones; never inline
   secrets — `integrations.json` enforces `vault://` prefixes for
   credential fields.
4. Run `python packages/tenant-config-schema/validate.py
   configs/tenants/<your-tenant>` and fix every error.
5. Open a PR; CI runs `make validate-all`.

---

## Adding a new config file type

1. Add a new JSON Schema (or extend an existing one) under `schemas/`.
2. Add a routing rule to `ROUTING_RULES` at the top of `validate.py`.
3. Add a fixture and assertion to `tests/test_validate.py`.
4. Update the routing table in this README.
