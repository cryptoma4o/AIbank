#!/usr/bin/env python3
"""Generate TypeScript types from schema.json.

Output: ``packages/domain-model/generated/ts/types.ts``

Mapping rules:
  * integer/number → number
  * string         → string
  * boolean        → boolean
  * array<T>       → T[]
  * enum           → union of string literals + ``as const`` value object
  * object         → ``interface``
  * date-time      → string (ISO 8601 — TypeScript has no first-class date)
"""

from __future__ import annotations

import sys
from pathlib import Path
from typing import Any

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


def _ts_type(body: dict[str, Any], all_defs: dict[str, Any]) -> str:
    if "$ref" in body:
        target = ref_name(body["$ref"])
        kind = classify_def(target, all_defs[target])
        if kind == "primitive":
            inner = all_defs[target]
            t = inner.get("type", "string")
            if t == "integer" or t == "number":
                return "number"
            if t == "boolean":
                return "boolean"
            return "string"
        return target
    t = body.get("type")
    if t == "string":
        return "string"
    if t in ("integer", "number"):
        return "number"
    if t == "boolean":
        return "boolean"
    if t == "array":
        return _ts_type(body.get("items", {"type": "string"}), all_defs) + "[]"
    if t == "object":
        if isinstance(body.get("additionalProperties"), dict):
            inner = _ts_type(body["additionalProperties"], all_defs)
            return f"Record<string, {inner}>"
        return "Record<string, unknown>"
    return "unknown"


def _emit_enum(name: str, body: dict[str, Any]) -> str:
    values = body["enum"]
    desc = body.get("description", "")
    out = []
    if desc:
        out.append(f"/** {desc} */")
    union = " | ".join(f'"{v}"' for v in values)
    out.append(f"export type {name} = {union};")
    out.append("")
    out.append(f"export const {name}Values = [")
    for v in values:
        out.append(f'  "{v}",')
    out.append("] as const;")
    return "\n".join(out)


def _emit_interface(name: str, body: dict[str, Any], all_defs: dict[str, Any]) -> str:
    desc = body.get("description", "")
    required = set(body.get("required", []))
    props = body.get("properties", {})
    out = []
    if desc:
        out.append(f"/** {desc} */")
    out.append(f"export interface {name} {{")
    for prop_name, prop_body in props.items():
        ts_type = _ts_type(prop_body, all_defs)
        opt = "" if prop_name in required else "?"
        prop_desc = prop_body.get("description")
        if prop_desc:
            out.append(f"  /** {prop_desc} */")
        out.append(f"  {prop_name}{opt}: {ts_type};")
    out.append("}")
    return "\n".join(out)


def render(schema: dict[str, Any]) -> str:
    all_defs = defs(schema)
    lines: list[str] = []
    lines.append(f"// {GENERATED_HEADER}")
    lines.append(f"// Source: schema.json ({schema.get('$id', 'unknown')})")
    lines.append("")

    for name, body in all_defs.items():
        if classify_def(name, body) == "enum":
            lines.append(_emit_enum(name, body))
            lines.append("")

    for name, body in all_defs.items():
        if classify_def(name, body) == "object":
            lines.append(_emit_interface(name, body, all_defs))
            lines.append("")

    return "\n".join(lines).rstrip() + "\n"


def main(out_path: Path | None = None) -> Path:
    schema = load_schema()
    out_path = out_path or (GENERATED_ROOT / "ts" / "types.ts")
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(render(schema), encoding="utf-8")
    return out_path


if __name__ == "__main__":
    written = main()
    print(f"wrote {written}")
