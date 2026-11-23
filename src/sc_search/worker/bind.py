"""Resolve $call_id.field placeholders from prior call results."""

from __future__ import annotations

import re
from typing import Any

_PLACEHOLDER = re.compile(r"^\$([A-Za-z0-9_]+)\.([A-Za-z0-9_]+)$")


def canonical_from_resolve(data: Any) -> str | None:
    if isinstance(data, list) and data:
        row = data[0]
        if isinstance(row, dict):
            name = row.get("canonical_name") or row.get("name")
            if isinstance(name, str) and name.strip():
                return name.strip()
    if isinstance(data, dict):
        name = data.get("canonical_name") or data.get("name")
        if isinstance(name, str) and name.strip():
            return name.strip()
    return None


def binding_value(call_id: str, field: str, results: dict[str, dict[str, Any]]) -> Any:
    row = results.get(call_id) or {}
    if field == "canonical":
        return canonical_from_resolve(row.get("data"))
    data = row.get("data")
    if isinstance(data, dict) and field in data:
        return data[field]
    return None


def bind_args(
    args: dict[str, Any],
    results: dict[str, dict[str, Any]],
) -> dict[str, Any]:
    out: dict[str, Any] = {}
    for key, val in args.items():
        if isinstance(val, str):
            m = _PLACEHOLDER.match(val.strip())
            if m:
                bound = binding_value(m.group(1), m.group(2), results)
                if bound is None:
                    out[key] = val
                else:
                    out[key] = bound
                continue
        out[key] = val
    return out
