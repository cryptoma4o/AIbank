"""Tests for the tenant-config validator.

Covers:
  * Each schema in schemas/ is a valid Draft-07 schema (lint).
  * configs/tenants/_template validates end-to-end.
  * A missing required field triggers a failure.
  * An invalid enum value triggers a failure.
"""

from __future__ import annotations

import json
import shutil
import textwrap
from pathlib import Path

import pytest
import yaml
from jsonschema import Draft7Validator

import validate as validator_mod  # noqa: E402  (sys.path tweak in conftest)

REPO_ROOT = Path(__file__).resolve().parents[3]
SCHEMAS = Path(__file__).resolve().parents[1] / "schemas"
TEMPLATE_TENANT = REPO_ROOT / "configs" / "tenants" / "_template"


def test_all_schemas_are_valid_draft7() -> None:
    schema_files = sorted(SCHEMAS.glob("*.json"))
    assert schema_files, "no schemas found in schemas/"
    for path in schema_files:
        data = json.loads(path.read_text(encoding="utf-8"))
        # Raises if the schema does not conform to Draft-07 meta-schema.
        Draft7Validator.check_schema(data)


def test_template_tenant_validates() -> None:
    assert TEMPLATE_TENANT.exists(), f"missing fixture: {TEMPLATE_TENANT}"
    rc = validator_mod.validate_tenant_dir(TEMPLATE_TENANT, verbose=False)
    assert rc == 0, "configs/tenants/_template did not validate"


def test_missing_required_field_fails(tmp_path: Path) -> None:
    # Copy the template and break it by removing a required field.
    fixture = tmp_path / "tenant"
    shutil.copytree(TEMPLATE_TENANT, fixture)
    tenant_yaml = fixture / "tenant.yaml"
    data = yaml.safe_load(tenant_yaml.read_text(encoding="utf-8"))
    data["tenant"].pop("bik")
    tenant_yaml.write_text(yaml.safe_dump(data, allow_unicode=True), encoding="utf-8")

    rc = validator_mod.validate_tenant_dir(fixture, verbose=False)
    assert rc == 1


def test_invalid_enum_fails(tmp_path: Path) -> None:
    fixture = tmp_path / "tenant"
    shutil.copytree(TEMPLATE_TENANT, fixture)
    tenant_yaml = fixture / "tenant.yaml"
    data = yaml.safe_load(tenant_yaml.read_text(encoding="utf-8"))
    data["tenant"]["deployment"]["mode"] = "kubernetes"  # not in enum
    tenant_yaml.write_text(yaml.safe_dump(data, allow_unicode=True), encoding="utf-8")

    rc = validator_mod.validate_tenant_dir(fixture, verbose=False)
    assert rc == 1


def test_invalid_pattern_fails(tmp_path: Path) -> None:
    fixture = tmp_path / "tenant"
    shutil.copytree(TEMPLATE_TENANT, fixture)
    tenant_yaml = fixture / "tenant.yaml"
    data = yaml.safe_load(tenant_yaml.read_text(encoding="utf-8"))
    data["tenant"]["bik"] = "12345"  # too short
    tenant_yaml.write_text(yaml.safe_dump(data, allow_unicode=True), encoding="utf-8")

    rc = validator_mod.validate_tenant_dir(fixture, verbose=False)
    assert rc == 1


def test_sla_thresholds_validate(tmp_path: Path) -> None:
    """Spot-check sla.json with a hand-built minimal sample."""
    fixture = tmp_path / "tenant"
    fixture.mkdir()
    sla = textwrap.dedent(
        """\
        schema_version: "1.0"
        targets:
          ip:
            auto_processing_minutes: 5
            total_minutes: 60
          llc:
            auto_processing_minutes: 15
            total_minutes: 240
          jsc:
            auto_processing_minutes: 30
            total_minutes: 480
        """
    )
    (fixture / "sla.yaml").write_text(sla, encoding="utf-8")
    rc = validator_mod.validate_tenant_dir(fixture, verbose=False)
    assert rc == 0


def test_route_unknown_path_returns_none() -> None:
    assert validator_mod.route(Path("AGENTS.md")) is None
    assert validator_mod.route(Path("README.md")) is None
    assert validator_mod.route(Path("tenant.yaml")) == "tenant.json"
    assert validator_mod.route(Path("ai/models.yaml")) == "ai.json"
    assert validator_mod.route(Path("workflows/llc.yaml")) == "workflow.json"
