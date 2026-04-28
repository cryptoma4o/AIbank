#!/usr/bin/env python3
"""CLI validator for tenant configuration directories.

Walks a tenant directory (e.g. ``configs/tenants/_template`` or
``configs/tenants/bank-alpha``), and validates each YAML file against the
JSON Schema in ``packages/tenant-config-schema/schemas/`` that matches
its location.

Usage:
    python validate.py <tenant_directory>

Exit codes:
    0 — all files validate
    1 — first validation failure (with clear path + message)
    2 — usage error or missing schema
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Iterable

try:
    import yaml  # type: ignore[import-untyped]
except ImportError as exc:  # pragma: no cover - import guard
    print(
        "ERROR: pyyaml is required. Install with `pip install pyyaml`.",
        file=sys.stderr,
    )
    raise SystemExit(2) from exc

try:
    import jsonschema
    from jsonschema import Draft7Validator
except ImportError as exc:  # pragma: no cover - import guard
    print(
        "ERROR: jsonschema is required. Install with `pip install 'jsonschema>=4'`.",
        file=sys.stderr,
    )
    raise SystemExit(2) from exc


SCHEMAS_DIR = Path(__file__).resolve().parent / "schemas"


# Mapping from a relative path inside the tenant directory to its schema.
# Order matters — the first match wins.
ROUTING_RULES: list[tuple[str, str]] = [
    ("tenant.yaml", "tenant.json"),
    ("sla.yaml", "sla.json"),
    ("branding/theme.json", "branding.json"),
    ("branding/content.yaml", "branding.json"),
    ("workflows/", "workflow.json"),
    ("risk-policy/rules.yaml", "risk-policy.json"),
    ("risk-policy/thresholds.yaml", "risk-policy.json"),
    ("risk-policy/blocked-okveds.yaml", "risk-policy.json"),
    ("integrations/abs.yaml", "integrations.json"),
    ("integrations/external.yaml", "integrations.json"),
    ("ai/models.yaml", "ai.json"),
    ("ai/prompts.yaml", "ai.json"),
]


def load_yaml_or_json(path: Path) -> Any:
    """Load a YAML or JSON file based on extension."""
    text = path.read_text(encoding="utf-8")
    if path.suffix == ".json":
        return json.loads(text)
    return yaml.safe_load(text)


def load_schema(name: str) -> dict[str, Any]:
    schema_path = SCHEMAS_DIR / name
    if not schema_path.exists():
        raise FileNotFoundError(f"schema not found: {schema_path}")
    with schema_path.open("r", encoding="utf-8") as fh:
        return json.load(fh)


def route(rel_path: Path) -> str | None:
    """Return the schema filename that applies, or None if unrouted."""
    rel = rel_path.as_posix()
    for prefix, schema_name in ROUTING_RULES:
        if prefix.endswith("/") and rel.startswith(prefix):
            return schema_name
        if rel == prefix:
            return schema_name
    return None


def iter_config_files(tenant_dir: Path) -> Iterable[Path]:
    """Yield each YAML/JSON file under ``tenant_dir`` (sorted, deterministic)."""
    for path in sorted(tenant_dir.rglob("*")):
        if path.is_file() and path.suffix in (".yaml", ".yml", ".json"):
            yield path


def format_error(path: Path, err: jsonschema.ValidationError) -> str:
    where = ".".join(str(p) for p in err.absolute_path) or "<root>"
    return (
        f"VALIDATION FAILED: {path}\n"
        f"  at: {where}\n"
        f"  message: {err.message}\n"
        f"  schema rule: {err.validator} = {err.validator_value!r}"
    )


def validate_tenant_dir(tenant_dir: Path, *, verbose: bool = False) -> int:
    """Validate every routable config file under ``tenant_dir``.

    Returns 0 on success, 1 on first failure.
    """
    if not tenant_dir.is_dir():
        print(f"ERROR: not a directory: {tenant_dir}", file=sys.stderr)
        return 2

    seen = 0
    for cfg_path in iter_config_files(tenant_dir):
        rel = cfg_path.relative_to(tenant_dir)
        schema_name = route(rel)
        if schema_name is None:
            if verbose:
                print(f"SKIP   {rel} (no schema route)")
            continue
        try:
            data = load_yaml_or_json(cfg_path)
        except (yaml.YAMLError, json.JSONDecodeError) as exc:
            print(f"PARSE FAILED: {cfg_path}\n  {exc}", file=sys.stderr)
            return 1
        try:
            schema = load_schema(schema_name)
        except FileNotFoundError as exc:
            print(f"ERROR: {exc}", file=sys.stderr)
            return 2
        validator = Draft7Validator(schema)
        errors = sorted(validator.iter_errors(data), key=lambda e: e.path)
        if errors:
            print(format_error(cfg_path, errors[0]), file=sys.stderr)
            return 1
        if verbose:
            print(f"OK     {rel}  →  {schema_name}")
        seen += 1

    if seen == 0:
        print(
            f"WARNING: no routable config files found under {tenant_dir}",
            file=sys.stderr,
        )
    else:
        print(f"OK: {seen} file(s) validated under {tenant_dir}")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tenant_dir", type=Path, help="Path to a tenant directory")
    parser.add_argument(
        "-v", "--verbose", action="store_true", help="Print each file/schema match"
    )
    args = parser.parse_args(argv)
    return validate_tenant_dir(args.tenant_dir, verbose=args.verbose)


if __name__ == "__main__":
    raise SystemExit(main())
