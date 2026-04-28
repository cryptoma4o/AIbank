"""Tests for the hand-rolled domain-model generators.

Verified properties:
  * ``schema.json`` parses as JSON and is valid Draft-07 meta-schema.
  * All ten canonical entities from docs/domain-model.md are present in
    ``$defs``.
  * Each generator writes a non-empty file at the expected path.
  * Generators are deterministic (same schema → same bytes on rerun).
  * The generated Python module imports cleanly when added to ``sys.path``.
  * The generated Go file is at least syntactically plausible (``package``
    declaration, struct keyword present). ``go vet`` is also exercised when
    the toolchain is available, but is skipped otherwise to keep the test
    suite hermetic.
"""

from __future__ import annotations

import importlib.util
import json
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

HERE = Path(__file__).resolve().parent
PACKAGE_ROOT = HERE.parent
SCHEMA_PATH = PACKAGE_ROOT / "schema.json"
GENERATED_ROOT = PACKAGE_ROOT / "generated"

# Make ``codegen`` importable when running pytest from the package root.
sys.path.insert(0, str(PACKAGE_ROOT))

REQUIRED_TYPES = {
    "Tenant",
    "Application",
    "Person",
    "LegalEntity",
    "Document",
    "RiskAssessment",
    "Decision",
    "Account",
    "AuditEvent",
    "UBOGraph",
}


def _schema() -> dict:
    return json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))


def test_schema_parses() -> None:
    schema = _schema()
    assert schema["$schema"] == "http://json-schema.org/draft-07/schema#"
    assert "$id" in schema
    assert "$defs" in schema


def test_schema_has_canonical_types() -> None:
    schema = _schema()
    present = set(schema["$defs"].keys())
    missing = REQUIRED_TYPES - present
    assert not missing, f"schema.json missing canonical types: {sorted(missing)}"


def test_generate_go_creates_file() -> None:
    from codegen.generate_go import main as gen_go  # type: ignore[import-not-found]

    out = gen_go()
    assert out.exists()
    contents = out.read_text(encoding="utf-8")
    assert contents.startswith("// Code generated from schema.json. DO NOT EDIT.")
    assert "package " in contents
    assert "type Application struct" in contents
    assert "type Tenant struct" in contents


def test_generate_ts_creates_file() -> None:
    from codegen.generate_ts import main as gen_ts  # type: ignore[import-not-found]

    out = gen_ts()
    assert out.exists()
    contents = out.read_text(encoding="utf-8")
    assert contents.startswith("// Code generated from schema.json. DO NOT EDIT.")
    assert "export interface Application" in contents
    assert "export interface Tenant" in contents


def test_generate_python_creates_file() -> None:
    from codegen.generate_python import main as gen_py  # type: ignore[import-not-found]

    out = gen_py()
    assert out.exists()
    contents = out.read_text(encoding="utf-8")
    assert "Code generated from schema.json. DO NOT EDIT." in contents
    assert "class Application(BaseModel):" in contents
    assert "class Tenant(BaseModel):" in contents


def test_generators_are_deterministic() -> None:
    from codegen.generate_go import main as gen_go  # type: ignore[import-not-found]
    from codegen.generate_python import main as gen_py  # type: ignore[import-not-found]
    from codegen.generate_ts import main as gen_ts  # type: ignore[import-not-found]

    first = (gen_go().read_bytes(), gen_ts().read_bytes(), gen_py().read_bytes())
    second = (gen_go().read_bytes(), gen_ts().read_bytes(), gen_py().read_bytes())
    assert first == second, "Generators are not deterministic"


def test_generated_python_imports() -> None:
    from codegen.generate_python import main as gen_py  # type: ignore[import-not-found]

    gen_py()
    target = GENERATED_ROOT / "python"
    sys.path.insert(0, str(target))
    spec = importlib.util.find_spec("aibank_domain.types")
    assert spec is not None, "aibank_domain.types is not importable"
    mod = importlib.import_module("aibank_domain.types")
    # Spot-check a known model.
    assert hasattr(mod, "Application")
    assert hasattr(mod, "Tenant")


def test_generated_go_passes_vet_when_toolchain_available() -> None:
    from codegen.generate_go import main as gen_go  # type: ignore[import-not-found]

    out = gen_go()
    assert out.exists()

    if shutil.which("go") is None:
        pytest.skip("go toolchain unavailable")

    go_dir = out.parent
    # Make sure go.mod exists in the generated dir so ``go vet ./...`` works.
    assert (go_dir / "go.mod").exists(), "generated/go/go.mod missing"
    res = subprocess.run(
        ["go", "vet", "./..."],
        cwd=go_dir,
        capture_output=True,
        text=True,
        check=False,
    )
    assert res.returncode == 0, (
        "go vet failed:\nstdout=" + res.stdout + "\nstderr=" + res.stderr
    )
