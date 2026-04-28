"""Shared helpers for the domain-model code generators.

The generators in this directory all read ``schema.json`` (JSON Schema
Draft 7) and emit deterministic, hand-rolled output for one target
language. We intentionally avoid third-party tools (datamodel-code-generator,
quicktype, json-schema-to-typescript, ...) so the codegen pipeline is
hermetic and reproducible from a stock Python install.

Determinism rules (enforced by tests):
  * No timestamps in generated headers.
  * Iteration order over ``$defs`` follows the order keys appear in
    ``schema.json`` (Python ``dict`` preserves insertion order on 3.7+).
  * Output bytes do not depend on environment variables.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

PACKAGE_ROOT = Path(__file__).resolve().parent.parent
SCHEMA_PATH = PACKAGE_ROOT / "schema.json"
GENERATED_ROOT = PACKAGE_ROOT / "generated"

# Header used by every generated artefact. Reviewers know not to hand-edit.
GENERATED_HEADER = "Code generated from schema.json. DO NOT EDIT."


def load_schema() -> dict[str, Any]:
    """Load and return the canonical schema as a Python dict."""
    with SCHEMA_PATH.open("r", encoding="utf-8") as fh:
        return json.load(fh)


def defs(schema: dict[str, Any]) -> dict[str, Any]:
    """Return the ``$defs`` block; raise if missing."""
    if "$defs" not in schema:
        raise RuntimeError("schema.json: missing $defs")
    return schema["$defs"]


# Names that are *types* (object schemas) rather than primitives or enums.
def classify_def(name: str, body: dict[str, Any]) -> str:
    """Classify a $defs entry: 'object' | 'enum' | 'primitive'."""
    if body.get("type") == "object" and "properties" in body:
        return "object"
    if "enum" in body and body.get("type") == "string":
        return "enum"
    return "primitive"


def ref_name(ref: str) -> str:
    """Resolve a ``$ref`` like ``#/$defs/PersonId`` to ``PersonId``."""
    if not ref.startswith("#/$defs/"):
        raise ValueError(f"Unsupported $ref: {ref}")
    return ref[len("#/$defs/") :]


def is_money_field(prop_name: str, body: dict[str, Any]) -> bool:
    """Heuristic — ``size_bytes`` and money fields are int64 in Go.

    For now any integer field is mapped to int64. The schema does not yet
    contain explicit kopecks fields but the contract in docs/domain-model.md
    section 1 is "money in kopecks → integer". int64 is the safe default.
    """
    return body.get("type") == "integer"
