#!/usr/bin/env python3
"""Generate Go types from schema.json.

Output: ``packages/domain-model/generated/go/types.go``

Mapping rules (mirrors docs/domain-model.md):
  * integer  → int64    (kopecks-safe)
  * number   → float64
  * string   → string  (with format=date-time → time.Time)
  * boolean  → bool
  * array<T> → []T
  * object   → map[string]interface{} (when free-form) or named struct
  * enum     → typed string + const block
"""

from __future__ import annotations

import sys
from pathlib import Path
from typing import Any

# Allow running both as ``python -m codegen.generate_go`` and as a script.
if __package__ in (None, ""):
    sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
    from codegen._common import (  # type: ignore[no-redef]
        GENERATED_HEADER,
        GENERATED_ROOT,
        classify_def,
        defs,
        load_schema,
        ref_name,
    )
else:  # pragma: no cover
    from ._common import (
        GENERATED_HEADER,
        GENERATED_ROOT,
        classify_def,
        defs,
        load_schema,
        ref_name,
    )


def _go_name(name: str) -> str:
    # All schema names are already PascalCase (Tenant, RiskAssessment …).
    return name


def _go_field_name(json_name: str) -> str:
    parts = json_name.split("_")
    out = []
    for p in parts:
        if not p:
            continue
        # Common Go-style initialisms.
        upper = p.upper()
        if upper in {"ID", "INN", "OGRN", "KPP", "BIK", "URL", "API", "SHA", "OCR", "S3", "IP", "UBO"}:
            out.append(upper)
        else:
            out.append(p[0].upper() + p[1:])
    return "".join(out) or json_name


_PRIMITIVE_TO_GO = {
    "string": "string",
    "integer": "int64",
    "number": "float64",
    "boolean": "bool",
}


def _go_type(body: dict[str, Any], all_defs: dict[str, Any]) -> str:
    if "$ref" in body:
        target = ref_name(body["$ref"])
        kind = classify_def(target, all_defs[target])
        if kind == "primitive":
            # Resolve to the primitive's Go type.
            inner = all_defs[target]
            if inner.get("format") == "date-time":
                return "time.Time"
            return _PRIMITIVE_TO_GO.get(inner.get("type", "string"), "string")
        if kind == "enum":
            return target
        return target  # struct
    t = body.get("type")
    if t == "string":
        if body.get("format") == "date-time":
            return "time.Time"
        return "string"
    if t == "integer":
        return "int64"
    if t == "number":
        return "float64"
    if t == "boolean":
        return "bool"
    if t == "array":
        return "[]" + _go_type(body.get("items", {"type": "string"}), all_defs)
    if t == "object":
        if body.get("additionalProperties") is True or isinstance(
            body.get("additionalProperties"), dict
        ):
            return "map[string]interface{}"
        return "map[string]interface{}"
    # any
    return "interface{}"


def _needs_pointer(prop_name: str, required: list[str]) -> bool:
    return prop_name not in required


def _emit_enum(name: str, body: dict[str, Any]) -> str:
    desc = body.get("description", "")
    out = []
    if desc:
        out.append(f"// {name} {desc}")
    out.append(f"type {name} string")
    out.append("")
    out.append("const (")
    for value in body["enum"]:
        const_suffix = "".join(p.capitalize() for p in str(value).split("_"))
        out.append(f'\t{name}{const_suffix} {name} = "{value}"')
    out.append(")")
    return "\n".join(out)


def _emit_struct(name: str, body: dict[str, Any], all_defs: dict[str, Any]) -> str:
    desc = body.get("description", "")
    required = body.get("required", [])
    props = body.get("properties", {})
    out = []
    if desc:
        out.append(f"// {name} {desc}")
    out.append(f"type {name} struct {{")
    for prop_name, prop_body in props.items():
        go_type = _go_type(prop_body, all_defs)
        json_tag = prop_name
        if _needs_pointer(prop_name, required):
            # Pointer for optional value types; arrays/maps stay nilable as-is.
            if not (go_type.startswith("[]") or go_type.startswith("map[") or go_type == "interface{}"):
                go_type = "*" + go_type
            json_tag += ",omitempty"
        field_name = _go_field_name(prop_name)
        prop_desc = prop_body.get("description")
        if prop_desc:
            out.append(f"\t// {field_name} {prop_desc}")
        out.append(f'\t{field_name} {go_type} `json:"{json_tag}"`')
    out.append("}")
    return "\n".join(out)


def _needs_time_import(all_defs: dict[str, Any]) -> bool:
    for body in all_defs.values():
        if body.get("format") == "date-time":
            return True
        for prop in body.get("properties", {}).values():
            if prop.get("format") == "date-time":
                return True
            if "$ref" in prop:
                target = ref_name(prop["$ref"])
                inner = all_defs.get(target, {})
                if inner.get("format") == "date-time":
                    return True
    return True  # Timestamp $def almost guaranteed to exist


def render(schema: dict[str, Any]) -> str:
    all_defs = defs(schema)
    title = schema.get("title", "domain")
    pkg = title.lower().replace("-", "")
    if pkg in {"", "domain"}:
        pkg = "domain"
    elif pkg.startswith("aibank"):
        pkg = "domain"

    lines: list[str] = []
    lines.append(f"// {GENERATED_HEADER}")
    lines.append(f"// Source: schema.json ({schema.get('$id', 'unknown')})")
    lines.append("")
    lines.append(f"package {pkg}")
    lines.append("")
    if _needs_time_import(all_defs):
        lines.append('import "time"')
        lines.append("")

    # Pass 1: enums (deterministic order = insertion order)
    for name, body in all_defs.items():
        if classify_def(name, body) == "enum":
            lines.append(_emit_enum(name, body))
            lines.append("")

    # Pass 2: structs (skip primitives — they map to primitive Go types inline)
    for name, body in all_defs.items():
        if classify_def(name, body) == "object":
            lines.append(_emit_struct(name, body, all_defs))
            lines.append("")

    return "\n".join(lines).rstrip() + "\n"


def main(out_path: Path | None = None) -> Path:
    schema = load_schema()
    out_path = out_path or (GENERATED_ROOT / "go" / "types.go")
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(render(schema), encoding="utf-8")
    return out_path


if __name__ == "__main__":
    written = main()
    print(f"wrote {written}")
