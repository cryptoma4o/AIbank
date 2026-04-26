# tenant-config-schema

JSON Schema definitions and CLI validator for AIbank tenant configuration files.

## Schemas

| Schema file | Validates |
|---|---|
| `schemas/tenant.schema.json` | `configs/tenants/<id>/tenant.yaml` |
| `schemas/risk-policy.schema.json` | `configs/tenants/<id>/risk-policy/thresholds.yaml` |
| `schemas/sla.schema.json` | `configs/tenants/<id>/sla.yaml` |
| `schemas/ai-models.schema.json` | `configs/tenants/<id>/ai/models.yaml` |

## Installation

```bash
pip install -e .
```

Or install dependencies directly:

```bash
pip install jsonschema pyyaml
```

## Validating a config file

```bash
python validate.py --schema schemas/tenant.schema.json --file configs/tenants/alfa-bank/tenant.yaml
python validate.py --schema schemas/sla.schema.json --file configs/tenants/alfa-bank/sla.yaml
python validate.py --schema schemas/risk-policy.schema.json --file configs/tenants/alfa-bank/risk-policy/thresholds.yaml
python validate.py --schema schemas/ai-models.schema.json --file configs/tenants/alfa-bank/ai/models.yaml
```

Exit code 0 = valid, exit code 1 = validation error.

## Schema constraints

### tenant.schema.json
- `tenant.id` must match `^[a-z0-9-]{3,50}$` (DNS-safe)
- `tenant.bik` must be exactly 9 digits
- `tenant.inn` must be exactly 10 digits (legal entity)
- `tenant.deployment.mode` must be one of: `saas`, `on_prem`, `hybrid`
- `tenant.status` defaults to `trial`

### risk-policy.schema.json
- `low_max`, `medium_max` range: 0–1000
- `auto_approve_max` range: 0–500
- Business constraint: `auto_approve_max <= low_max <= medium_max` (enforced in application logic)

### sla.schema.json
- Required entity types: `ip` (sole trader), `llc` (ООО), `jsc` (АО)
- Each type requires `auto_processing_minutes` and `total_minutes`

### ai-models.schema.json
- Required roles: `document_parsing`, `reconciliation`, `ubo_tracing`, `risk_explanation`, `conversational`, `rag`
- Each role requires `model_name` (alias in llm-gateway), `rate_limit_rpm`, `cost_alert_usd_per_day`
