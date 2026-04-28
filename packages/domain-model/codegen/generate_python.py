#!/usr/bin/env python3
"""Generate Pydantic v2 models from schema.json.

Output: ``packages/domain-model/generated/python/aibank_domain/types.py``

Mapping rules:
  * integer        → int
  * number         → float
  * string         → str
  * date-time      → datetime
  * boolean        → bool
  * array<T>       → list[T]
  * object         → ``BaseModel`` subclass
  * enum           → ``str, Enum`` subclass
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


_PRIMITIVE_TO_PY = {
    "string": "str",
    "integer": "int",
    "number": "float",
    "boolean": "bool",
}


def _py_type(body: dict[str, Any], all_defs: dict[str, Any]) -> str:
    if "$ref" in body:
        target = ref_name(body["$ref"])
        kind = classify_def(target, all_defs[target])
        if kind == "primitive":
            inner = all_defs[target]
            if inner.get("format") == "date-time":
                return "datetime"
            return _PRIMITIVE_TO_PY.get(inner.get("type", "string"), "str")
        return target
    t = body.get("type")
    if t == "string":
        if body.get("format") == "date-time":
            return "datetime"
        return "str"
    if t == "integer":
        return "int"
    if t == "number":
        return "float"
    if t == "boolean":
        return "bool"
    if t == "array":
        return f"list[{_py_type(body.get('items', {'type': 'string'}), all_defs)}]"
    if t == "object":
        if isinstance(body.get("additionalProperties"), dict):
            inner = _py_type(body["additionalProperties"], all_defs)
            return f"dict[str, {inner}]"
        return "dict[str, Any]"
    return "Any"


def _emit_enum(name: str, body: dict[str, Any]) -> str:
    desc = body.get("description", "")
    out = [f"class {name}(str, Enum):"]
    if desc:
        out.append(f'    """{desc}"""')
    out.append("")
    for value in body["enum"]:
        member = str(value).upper()
        # Python identifiers cannot start with a digit.
        if member and member[0].isdigit():
            member = "_" + member
        out.append(f'    {member} = "{value}"')
    return "\n".join(out)


def _emit_model(name: str, body: dict[str, Any], all_defs: dict[str, Any]) -> str:
    desc = body.get("description", "")
    required = set(body.get("required", []))
    props = body.get("properties", {})
    out = [f"class {name}(BaseModel):"]
    if desc:
        out.append(f'    """{desc}"""')
        out.append("")
    if not props:
        out.append("    pass")
        return "\n".join(out)
    for prop_name, prop_body in props.items():
        py_type = _py_type(prop_body, all_defs)
        if prop_name not in required:
            py_type = f"{py_type} | None"
            default = " = None"
        else:
            default = ""
        out.append(f"    {prop_name}: {py_type}{default}")
    return "\n".join(out)


def render(schema: dict[str, Any]) -> str:
    all_defs = defs(schema)
    lines: list[str] = []
    lines.append(f'"""{GENERATED_HEADER}')
    lines.append("")
    lines.append(f"Source: schema.json ({schema.get('$id', 'unknown')})")
    lines.append('"""')
    lines.append("")
    lines.append("from __future__ import annotations")
    lines.append("")
    lines.append("from datetime import datetime")
    lines.append("from enum import Enum")
    lines.append("from typing import Any")
    lines.append("")
    lines.append("from pydantic import BaseModel")
    lines.append("")
    lines.append("")

    for name, body in all_defs.items():
        if classify_def(name, body) == "enum":
            lines.append(_emit_enum(name, body))
            lines.append("")
            lines.append("")

    for name, body in all_defs.items():
        if classify_def(name, body) == "object":
            lines.append(_emit_model(name, body, all_defs))
            lines.append("")
            lines.append("")

    # __all__ for explicit re-export
    exported = [n for n, b in all_defs.items() if classify_def(n, b) in ("enum", "object")]
    lines.append("__all__ = [")
    for n in exported:
        lines.append(f'    "{n}",')
    lines.append("]")
    return "\n".join(lines).rstrip() + "\n"


def main(out_path: Path | None = None) -> Path:
    schema = load_schema()
    out_path = out_path or (
        GENERATED_ROOT / "python" / "aibank_domain" / "types.py"
    )
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(render(schema), encoding="utf-8")
    # Ensure __init__.py also exists so the package is importable.
    init = out_path.parent / "__init__.py"
    if not init.exists():
        init.write_text(
            f'"""{GENERATED_HEADER}"""\n'
            "from .types import *  # noqa: F401,F403\n",
            encoding="utf-8",
        )
    return out_path


if __name__ == "__main__":
    written = main()
    print(f"wrote {written}")
